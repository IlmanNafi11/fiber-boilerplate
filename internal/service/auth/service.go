package auth

import (
	"context"
	"errors"
	"time"
	"unicode"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	usermodel "github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// RefreshTokenRepo defines the interface for refresh token persistence.
type RefreshTokenRepo interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*aservice.RefreshToken, error)
	Create(ctx context.Context, t *aservice.RefreshToken) error
	RevokeBySessionID(ctx context.Context, sessionID string) error
	RevokeByUserID(ctx context.Context, userID string) error
	RevokeWithTx(ctx context.Context, tx pgx.Tx, id string, graceUntil *time.Time) error
	CreateWithTx(ctx context.Context, tx pgx.Tx, t *aservice.RefreshToken) error
}

// SessionRepo defines the interface for session persistence.
type SessionRepo interface {
	Create(ctx context.Context, s *aservice.Session) error
	GetByID(ctx context.Context, id string) (*aservice.Session, error)
	RevokeByID(ctx context.Context, id string) error
	RevokeByUserID(ctx context.Context, userID string) error
}

// EmailVerificationTokenRepo defines the interface for email verification token persistence.
type EmailVerificationTokenRepo interface {
	Create(ctx context.Context, t *aservice.EmailVerificationToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*aservice.EmailVerificationToken, error)
	GetActiveByUserID(ctx context.Context, userID string) (*aservice.EmailVerificationToken, error)
	MarkUsed(ctx context.Context, id string) error
	MarkUsedByUserID(ctx context.Context, userID string) error
}

// PasswordResetTokenRepo defines the interface for password reset token persistence.
type PasswordResetTokenRepo interface {
	Create(ctx context.Context, t *aservice.PasswordResetToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*aservice.PasswordResetToken, error)
	MarkUsed(ctx context.Context, id string) error
	MarkUsedByUserID(ctx context.Context, userID string) error
}

// EmailSender defines the interface for sending transactional emails.
type EmailSender interface {
	SendVerificationEmail(ctx context.Context, to, token string) error
	SendPasswordResetEmail(ctx context.Context, to, token string) error
	SendPasswordChangedNotification(ctx context.Context, to string) error
}

type AuthService struct {
	userRepo                   *userrepo.UserRepository
	sessionRepo                SessionRepo
	refreshTokenRepo           RefreshTokenRepo
	emailVerificationTokenRepo EmailVerificationTokenRepo
	passwordResetTokenRepo     PasswordResetTokenRepo
	tokenHelper                *TokenHelper
	emailSender                EmailSender
	cfg                        config.AuthConfig
	emailCfg                   config.EmailConfig
	logger                     *zap.Logger
	pool                       *pgxpool.Pool
}

func NewAuthService(
	userRepo *userrepo.UserRepository,
	sessionRepo SessionRepo,
	refreshTokenRepo RefreshTokenRepo,
	emailVerificationTokenRepo EmailVerificationTokenRepo,
	passwordResetTokenRepo PasswordResetTokenRepo,
	tokenHelper *TokenHelper,
	emailSender EmailSender,
	cfg config.AuthConfig,
	emailCfg config.EmailConfig,
	logger *zap.Logger,
	pool *pgxpool.Pool,
) *AuthService {
	return &AuthService{
		userRepo:                   userRepo,
		sessionRepo:                sessionRepo,
		refreshTokenRepo:           refreshTokenRepo,
		emailVerificationTokenRepo: emailVerificationTokenRepo,
		passwordResetTokenRepo:     passwordResetTokenRepo,
		tokenHelper:                tokenHelper,
		emailSender:                emailSender,
		cfg:                        cfg,
		emailCfg:                   emailCfg,
		logger:                     logger,
		pool:                       pool,
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

	// Handle email verification (D-10, D-13, EML-01)
	if s.emailCfg.VerificationEnabled {
		plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
		if err != nil {
			s.logger.Error("failed to generate verification token", zap.Error(err))
		} else {
			verificationToken := &aservice.EmailVerificationToken{
				UserID:    u.ID,
				TokenHash: tokenHash,
				ExpiresAt: time.Now().Add(s.emailCfg.VerificationTokenTTL),
			}
			if err := s.emailVerificationTokenRepo.Create(ctx, verificationToken); err != nil {
				s.logger.Error("failed to store verification token", zap.Error(err))
			} else {
				go func() {
					if err := s.emailSender.SendVerificationEmail(context.Background(), u.Email, plainToken); err != nil {
						s.logger.Error("failed to send verification email",
							zap.String("user_id", u.ID),
							zap.Error(err),
						)
					}
				}()
			}
		}
	} else {
		if err := s.userRepo.UpdateEmailVerifiedAt(ctx, u.ID); err == nil {
			now := time.Now()
			u.EmailVerifiedAt = &now
		}
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

	// Check email verification when enabled (D-12, EML-04)
	if s.emailCfg.VerificationEnabled && u.EmailVerifiedAt == nil {
		return nil, errx.Forbidden("email verification required. Please check your email or request a new verification link")
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
		return errx.Unauthorized("invalid refresh token")
	}

	if token.RevokedAt != nil {
		return errx.Unauthorized("invalid refresh token")
	}

	session, err := s.sessionRepo.GetByID(ctx, token.SessionID)
	if err != nil {
		return errx.Unauthorized("invalid refresh token")
	}
	if session.RevokedAt != nil {
		return errx.Unauthorized("invalid refresh token")
	}

	if err := s.sessionRepo.RevokeByID(ctx, token.SessionID); err != nil {
		return errx.Internal("logout failed", err.Error())
	}

	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, token string) error {
	tokenHash := s.tokenHelper.ParseRefreshTokenHash(token)

	verificationToken, err := s.emailVerificationTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errx.ErrNotFound) {
			return errx.BadRequest("invalid or expired verification token")
		}
		return errx.Internal("token lookup failed", err.Error())
	}

	if verificationToken.UsedAt != nil {
		return errx.BadRequest("verification token already used")
	}

	if time.Now().After(verificationToken.ExpiresAt) {
		return errx.BadRequest("verification token has expired")
	}

	if err := s.emailVerificationTokenRepo.MarkUsed(ctx, verificationToken.ID); err != nil {
		return errx.Internal("token invalidation failed", err.Error())
	}

	if err := s.userRepo.UpdateEmailVerifiedAt(ctx, verificationToken.UserID); err != nil {
		return errx.Internal("email verification update failed", err.Error())
	}

	return nil
}

func (s *AuthService) ResendVerification(ctx context.Context, email string) error {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}

	if u.EmailVerifiedAt != nil {
		return nil
	}

	_ = s.emailVerificationTokenRepo.MarkUsedByUserID(ctx, u.ID)

	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return errx.Internal("token generation failed", err.Error())
	}

	verificationToken := &aservice.EmailVerificationToken{
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.emailCfg.VerificationTokenTTL),
	}
	if err := s.emailVerificationTokenRepo.Create(ctx, verificationToken); err != nil {
		return errx.Internal("token storage failed", err.Error())
	}

	go func() {
		if err := s.emailSender.SendVerificationEmail(context.Background(), u.Email, plainToken); err != nil {
			s.logger.Error("failed to send verification email",
				zap.String("user_id", u.ID),
				zap.Error(err),
			)
		}
	}()

	return nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, email string) error {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}

	_ = s.passwordResetTokenRepo.MarkUsedByUserID(ctx, u.ID)

	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return errx.Internal("token generation failed", err.Error())
	}

	resetToken := &aservice.PasswordResetToken{
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.emailCfg.PasswordResetTokenTTL),
	}
	if err := s.passwordResetTokenRepo.Create(ctx, resetToken); err != nil {
		return errx.Internal("token storage failed", err.Error())
	}

	go func() {
		if err := s.emailSender.SendPasswordResetEmail(context.Background(), u.Email, plainToken); err != nil {
			s.logger.Error("failed to send password reset email",
				zap.String("user_id", u.ID),
				zap.Error(err),
			)
		}
	}()

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, token string, newPassword string) error {
	tokenHash := s.tokenHelper.ParseRefreshTokenHash(token)

	resetToken, err := s.passwordResetTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errx.ErrNotFound) {
			return errx.BadRequest("invalid or expired reset token")
		}
		return errx.Internal("token lookup failed", err.Error())
	}

	if resetToken.UsedAt != nil {
		return errx.BadRequest("reset token already used")
	}

	if time.Now().After(resetToken.ExpiresAt) {
		return errx.BadRequest("reset token has expired")
	}

	hasLetter := false
	hasDigit := false
	for _, c := range newPassword {
		if unicode.IsLetter(c) {
			hasLetter = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errx.BadRequest("password must contain at least one letter and one digit")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errx.Internal("password hashing failed", err.Error())
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errx.Internal("transaction begin failed", err.Error())
	}
	defer tx.Rollback(ctx)

	if err := s.passwordResetTokenRepo.MarkUsed(ctx, resetToken.ID); err != nil {
		return errx.Internal("token invalidation failed", err.Error())
	}

	if err := s.userRepo.UpdatePassword(ctx, resetToken.UserID, string(hash)); err != nil {
		return errx.Internal("password update failed", err.Error())
	}

	if err := s.sessionRepo.RevokeByUserID(ctx, resetToken.UserID); err != nil {
		return errx.Internal("session revocation failed", err.Error())
	}

	if err := s.refreshTokenRepo.RevokeByUserID(ctx, resetToken.UserID); err != nil {
		return errx.Internal("refresh token revocation failed", err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return errx.Internal("transaction commit failed", err.Error())
	}

	u, err := s.userRepo.GetByID(ctx, resetToken.UserID)
	if err == nil {
		go func() {
			if err := s.emailSender.SendPasswordChangedNotification(context.Background(), u.Email); err != nil {
				s.logger.Error("failed to send password change notification",
					zap.String("user_id", u.ID),
					zap.Error(err),
				)
			}
		}()
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
