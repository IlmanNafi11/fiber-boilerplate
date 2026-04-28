package auth

import (
	"context"
	"errors"
	"time"
	"unicode"

	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	usermodel "github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"go.uber.org/zap"
)

// RefreshTokenRepo defines the interface for refresh token persistence.
type RefreshTokenRepo interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*aservice.RefreshToken, error)
	Create(ctx context.Context, t *aservice.RefreshToken) error
	RevokeBySessionID(ctx context.Context, sessionID string) error
	RevokeWithTx(ctx context.Context, tx pgx.Tx, id string, graceUntil *time.Time) error
	CreateWithTx(ctx context.Context, tx pgx.Tx, t *aservice.RefreshToken) error
}

// SessionRepo defines the interface for session persistence.
type SessionRepo interface {
	Create(ctx context.Context, s *aservice.Session) error
	GetByID(ctx context.Context, id string) (*aservice.Session, error)
	RevokeByID(ctx context.Context, id string) error
}

type AuthService struct {
	userRepo         *userrepo.UserRepository
	sessionRepo      SessionRepo
	refreshTokenRepo RefreshTokenRepo
	tokenHelper      *TokenHelper
	cfg              config.AuthConfig
	logger           *zap.Logger
	pool             *pgxpool.Pool
}

func NewAuthService(
	userRepo *userrepo.UserRepository,
	sessionRepo SessionRepo,
	refreshTokenRepo RefreshTokenRepo,
	tokenHelper *TokenHelper,
	cfg config.AuthConfig,
	logger *zap.Logger,
	pool *pgxpool.Pool,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		sessionRepo:      sessionRepo,
		refreshTokenRepo: refreshTokenRepo,
		tokenHelper:      tokenHelper,
		cfg:              cfg,
		logger:           logger,
		pool:             pool,
	}
}

func (s *AuthService) Register(ctx context.Context, req *authdto.RegisterRequest) (*usermodel.User, error) {
	if !s.cfg.RegistrationEnabled {
		return nil, errx.Forbidden("registration is currently disabled")
	}

	// Supplementary password validation: at least 1 letter and 1 digit
	hasLetter := false
	hasDigit := false
	for _, c := range req.Password {
		if unicode.IsLetter(c) {
			hasLetter = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return nil, errx.BadRequest("password must contain at least one letter and one digit")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errx.Internal("password hashing failed", err.Error())
	}

	u := &usermodel.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         user.RoleUser,
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, u); err != nil {
		if errors.Is(err, errx.ErrDuplicate) {
			return nil, errx.Conflict("email already registered")
		}
		return nil, errx.Internal("user creation failed", err.Error())
	}

	u.PasswordHash = ""
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, req *authdto.LoginRequest, userAgent, ip string) (*aservice.TokenResponse, error) {
	u, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, errx.Unauthorized("invalid email or password")
		}
		return nil, errx.Internal("login failed", err.Error())
	}

	if !u.IsActive {
		return nil, errx.Forbidden("account is disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errx.Unauthorized("invalid email or password")
	}

	// Create session
	session := &aservice.Session{
		UserID:    u.ID,
		UserAgent: userAgent,
		IPAddress: ip,
		ExpiresAt: time.Now().Add(s.tokenHelper.RefreshTTL()),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, errx.Internal("session creation failed", err.Error())
	}

	// Generate refresh token
	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return nil, errx.Internal("token generation failed", err.Error())
	}

	refreshToken := &aservice.RefreshToken{
		SessionID: session.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.tokenHelper.RefreshTTL()),
	}
	if err := s.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return nil, errx.Internal("refresh token storage failed", err.Error())
	}

	// Generate access token
	accessToken, err := s.tokenHelper.GenerateAccessToken(u.ID, u.Role, session.ID)
	if err != nil {
		return nil, errx.Internal("access token generation failed", err.Error())
	}

	return &aservice.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: plainToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenHelper.AccessTTLSeconds(),
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, plainToken string) (*aservice.TokenResponse, error) {
	tokenHash := s.tokenHelper.ParseRefreshTokenHash(plainToken)

	token, err := s.refreshTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errx.ErrNotFound) {
			return nil, errx.Unauthorized("invalid refresh token")
		}
		return nil, errx.Internal("token lookup failed", err.Error())
	}

	// Check token expiry
	if time.Now().After(token.ExpiresAt) {
		return nil, errx.Unauthorized("refresh token expired")
	}

	// Check session
	session, err := s.sessionRepo.GetByID(ctx, token.SessionID)
	if err != nil {
		return nil, errx.Unauthorized("invalid refresh token")
	}
	if session.RevokedAt != nil {
		return nil, errx.Unauthorized("session has been revoked")
	}

	// Reuse detection: token already revoked
	if token.RevokedAt != nil {
		if token.GraceUntil != nil && time.Now().Before(*token.GraceUntil) {
			// Within grace period — concurrent refresh, issue new token pair
			// without revoking the session
			newPlain, newHash, err := s.tokenHelper.GenerateRefreshToken()
			if err != nil {
				return nil, errx.Internal("token generation failed", err.Error())
			}

			newToken := &aservice.RefreshToken{
				SessionID: session.ID,
				TokenHash: newHash,
				ExpiresAt: time.Now().Add(s.tokenHelper.RefreshTTL()),
			}
			if err := s.refreshTokenRepo.Create(ctx, newToken); err != nil {
				return nil, errx.Internal("refresh token storage failed", err.Error())
			}

			// Fetch user for access token
			u, err := s.userRepo.GetByID(ctx, session.UserID)
			if err != nil {
				return nil, errx.Internal("user lookup failed", err.Error())
			}

			accessToken, err := s.tokenHelper.GenerateAccessToken(u.ID, u.Role, session.ID)
			if err != nil {
				return nil, errx.Internal("access token generation failed", err.Error())
			}

			return &aservice.TokenResponse{
				AccessToken:  accessToken,
				RefreshToken: newPlain,
				TokenType:    "Bearer",
				ExpiresIn:    s.tokenHelper.AccessTTLSeconds(),
			}, nil
		}

		// Outside grace period — reuse detected, revoke entire session
		_ = s.sessionRepo.RevokeByID(ctx, token.SessionID)
		_ = s.refreshTokenRepo.RevokeBySessionID(ctx, token.SessionID)
		return nil, errx.Unauthorized("token reuse detected, session revoked")
	}

	// Normal rotation: transactional revoke old + create new
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, errx.Internal("transaction begin failed", err.Error())
	}
	defer tx.Rollback(ctx)

	graceTime := time.Now().Add(s.tokenHelper.GracePeriod())
	if err := s.refreshTokenRepo.RevokeWithTx(ctx, tx, token.ID, &graceTime); err != nil {
		return nil, errx.Internal("token revocation failed", err.Error())
	}

	newPlain, newHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return nil, errx.Internal("token generation failed", err.Error())
	}

	newToken := &aservice.RefreshToken{
		SessionID: session.ID,
		TokenHash: newHash,
		ExpiresAt: time.Now().Add(s.tokenHelper.RefreshTTL()),
	}
	if err := s.refreshTokenRepo.CreateWithTx(ctx, tx, newToken); err != nil {
		return nil, errx.Internal("refresh token creation failed", err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, errx.Internal("transaction commit failed", err.Error())
	}

	// Fetch user for access token
	u, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, errx.Internal("user lookup failed", err.Error())
	}

	accessToken, err := s.tokenHelper.GenerateAccessToken(u.ID, u.Role, session.ID)
	if err != nil {
		return nil, errx.Internal("access token generation failed", err.Error())
	}

	return &aservice.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newPlain,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenHelper.AccessTTLSeconds(),
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, plainToken string) error {
	tokenHash := s.tokenHelper.ParseRefreshTokenHash(plainToken)

	token, err := s.refreshTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		// Don't reveal whether token exists — return generic error
		return errx.Unauthorized("invalid refresh token")
	}

	// Reject if the refresh token itself was already revoked
	if token.RevokedAt != nil {
		return errx.Unauthorized("invalid refresh token")
	}

	// Check if the session is already revoked (e.g., from a previous logout)
	session, err := s.sessionRepo.GetByID(ctx, token.SessionID)
	if err != nil {
		return errx.Unauthorized("invalid refresh token")
	}
	if session.RevokedAt != nil {
		return errx.Unauthorized("invalid refresh token")
	}

	// Revoke entire session
	if err := s.sessionRepo.RevokeByID(ctx, token.SessionID); err != nil {
		return errx.Internal("logout failed", err.Error())
	}

	return nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (*aservice.UserResponse, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, errx.Unauthorized("user not found")
		}
		return nil, errx.Internal("user lookup failed", err.Error())
	}

	return &aservice.UserResponse{
		ID:              u.ID,
		Email:           u.Email,
		Role:            u.Role,
		IsActive:        u.IsActive,
		EmailVerifiedAt: u.EmailVerifiedAt,
		CreatedAt:       u.CreatedAt,
	}, nil
}
