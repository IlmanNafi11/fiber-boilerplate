package auth

import (
	"context"
	"errors"
	"time"
	"unicode"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	emaildomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	usermodel "github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	"github.com/ilmannafi/fiber-boilerplate/internal/service/email"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// RefreshTokenRepo defines the interface for refresh token persistence.
type RefreshTokenRepo interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (*authdto.RefreshToken, error)
	Create(ctx context.Context, t *authdto.RefreshToken) error
	RevokeBySessionID(ctx context.Context, sessionID string) error
	RevokeWithTx(ctx context.Context, tx pgx.Tx, id string, graceUntil *time.Time) error
	CreateWithTx(ctx context.Context, tx pgx.Tx, t *authdto.RefreshToken) error
	RevokeByUserIDWithTx(ctx context.Context, tx pgx.Tx, userID string) error
}

// SessionRepo defines the interface for session persistence.
type SessionRepo interface {
	Create(ctx context.Context, s *authdto.Session) error
	GetByID(ctx context.Context, id string) (*authdto.Session, error)
	RevokeByID(ctx context.Context, id string) error
	RevokeByUserIDWithTx(ctx context.Context, tx pgx.Tx, userID string) error
}

// EmailVerificationTokenRepo defines the interface for email verification token persistence.
type EmailVerificationTokenRepo interface {
	Create(ctx context.Context, t *authdto.EmailVerificationToken) error
	CreateWithTx(ctx context.Context, tx pgx.Tx, t *authdto.EmailVerificationToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*authdto.EmailVerificationToken, error)
	MarkUsed(ctx context.Context, id string) error
	MarkUsedByUserID(ctx context.Context, userID string) error
	MarkUsedByUserIDWithTx(ctx context.Context, tx pgx.Tx, userID string) error
}

// PasswordResetTokenRepo defines the interface for password reset token persistence.
type PasswordResetTokenRepo interface {
	Create(ctx context.Context, t *authdto.PasswordResetToken) error
	CreateWithTx(ctx context.Context, tx pgx.Tx, t *authdto.PasswordResetToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*authdto.PasswordResetToken, error)
	MarkUsedByUserID(ctx context.Context, userID string) error
	MarkUsedByUserIDWithTx(ctx context.Context, tx pgx.Tx, userID string) error
	MarkUsedWithTx(ctx context.Context, tx pgx.Tx, id string) error
}

// OutboxRepo enqueues durable email events within a caller-provided transaction.
type OutboxRepo interface {
	EnqueueWithTx(ctx context.Context, tx pgx.Tx, e *emaildomain.OutboxEvent) error
}

type AuthService struct {
	userRepo                   *userrepo.UserRepository
	sessionRepo                SessionRepo
	refreshTokenRepo           RefreshTokenRepo
	emailVerificationTokenRepo EmailVerificationTokenRepo
	passwordResetTokenRepo     PasswordResetTokenRepo
	outboxRepo                 OutboxRepo
	tokenHelper                *TokenHelper
	emailSender                email.EmailSender
	cfg                        config.AuthConfig
	emailCfg                   config.EmailConfig
	logger                     *zap.Logger
	pool                       *pgxpool.Pool
	now                        func() time.Time
}

func NewAuthService(
	userRepo *userrepo.UserRepository,
	sessionRepo SessionRepo,
	refreshTokenRepo RefreshTokenRepo,
	emailVerificationTokenRepo EmailVerificationTokenRepo,
	passwordResetTokenRepo PasswordResetTokenRepo,
	outboxRepo OutboxRepo,
	tokenHelper *TokenHelper,
	emailSender email.EmailSender,
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
		outboxRepo:                 outboxRepo,
		tokenHelper:                tokenHelper,
		emailSender:                emailSender,
		cfg:                        cfg,
		emailCfg:                   emailCfg,
		logger:                     logger,
		pool:                       pool,
		now:                        time.Now,
	}
}

// clock returns the current time via the injected clock, falling back to
// time.Now when the service was built without one (e.g. struct-literal tests).
func (s *AuthService) clock() time.Time {
	if fn := s.now; fn != nil {
		return fn()
	}
	return time.Now()
}

func (s *AuthService) Register(ctx context.Context, req *authdto.RegisterRequest) (*usermodel.User, error) {
	if !s.cfg.RegistrationEnabled {
		return nil, errx.Forbidden("registration is currently disabled")
	}

	if !passwordHasLetterAndDigit(req.Password) {
		return nil, errx.BadRequest("password must contain at least one letter and one digit")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errx.Internal("password hashing failed", err.Error())
	}

	u := &usermodel.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         usermodel.RoleUser,
		IsActive:     true,
	}

	if err := s.userRepo.Create(ctx, u); err != nil {
		if errors.Is(err, errx.ErrDuplicate) {
			return nil, errx.Conflict("email already registered")
		}
		return nil, errx.Internal("user creation failed", err.Error())
	}

	// Handle email verification (D-10, D-13, EML-01). The token and its outbox
	// event commit together so a durable dispatch is guaranteed for every
	// verification token that reaches persistence.
	if s.emailCfg.VerificationEnabled {
		plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
		if err != nil {
			s.logger.Error("failed to generate verification token", zap.Error(err))
		} else {
			verificationToken := &authdto.EmailVerificationToken{
				UserID:    u.ID,
				TokenHash: tokenHash,
				ExpiresAt: s.clock().Add(s.emailCfg.VerificationTokenTTL),
			}
			err := s.withTx(ctx, func(tx pgx.Tx) error {
				if err := s.emailVerificationTokenRepo.CreateWithTx(ctx, tx, verificationToken); err != nil {
					return err
				}
				return s.enqueueTokenEmail(ctx, tx, emaildomain.EventTypeVerification, u.Email, plainToken)
			})
			if err != nil {
				s.logger.Error("failed to persist verification token", zap.String("user_id", u.ID), zap.Error(err))
			}
		}
	} else {
		if err := s.userRepo.UpdateEmailVerifiedAt(ctx, u.ID); err == nil {
			now := s.clock()
			u.EmailVerifiedAt = &now
		}
	}

	u.PasswordHash = ""
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, req *authdto.LoginRequest, userAgent, ip string) (*authdto.TokenResponse, error) {
	u, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, usermodel.ErrUserNotFound) {
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
	session := &authdto.Session{
		UserID:    u.ID,
		UserAgent: userAgent,
		IPAddress: ip,
		ExpiresAt: s.clock().Add(s.tokenHelper.RefreshTTL()),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, errx.Internal("session creation failed", err.Error())
	}

	plainToken, refreshToken, err := s.newRefreshToken(session.ID)
	if err != nil {
		return nil, errx.Internal("token generation failed", err.Error())
	}
	if err := s.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return nil, errx.Internal("refresh token storage failed", err.Error())
	}

	return s.buildTokenResponse(u.ID, u.Role, session.ID, plainToken)
}

func (s *AuthService) RefreshToken(ctx context.Context, plainToken string) (*authdto.TokenResponse, error) {
	tokenHash := s.tokenHelper.ParseRefreshTokenHash(plainToken)

	token, err := s.refreshTokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errx.ErrNotFound) {
			return nil, errx.Unauthorized("invalid refresh token")
		}
		return nil, errx.Internal("token lookup failed", err.Error())
	}

	// Check token expiry
	if s.clock().After(token.ExpiresAt) {
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
		if token.GraceUntil != nil && s.clock().Before(*token.GraceUntil) {
			newPlain, newToken, err := s.newRefreshToken(session.ID)
			if err != nil {
				return nil, errx.Internal("token generation failed", err.Error())
			}
			if err := s.refreshTokenRepo.Create(ctx, newToken); err != nil {
				return nil, errx.Internal("refresh token storage failed", err.Error())
			}

			u, err := s.userRepo.GetByID(ctx, session.UserID)
			if err != nil {
				return nil, errx.Internal("user lookup failed", err.Error())
			}

			return s.buildTokenResponse(u.ID, u.Role, session.ID, newPlain)
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
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			s.logger.Debug("deferred rollback after commit", zap.Error(err))
		}
	}()

	graceTime := s.clock().Add(s.tokenHelper.GracePeriod())
	if err := s.refreshTokenRepo.RevokeWithTx(ctx, tx, token.ID, &graceTime); err != nil {
		return nil, errx.Internal("token revocation failed", err.Error())
	}

	newPlain, newToken, err := s.newRefreshToken(session.ID)
	if err != nil {
		return nil, errx.Internal("token generation failed", err.Error())
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

	return s.buildTokenResponse(u.ID, u.Role, session.ID, newPlain)
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

	if s.clock().After(verificationToken.ExpiresAt) {
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

	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return errx.Internal("token generation failed", err.Error())
	}

	verificationToken := &authdto.EmailVerificationToken{
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: s.clock().Add(s.emailCfg.VerificationTokenTTL),
	}

	// Invalidate outstanding tokens, persist the new one, and enqueue its email
	// atomically so a resend never leaves a token without a durable dispatch.
	if err := s.withTx(ctx, func(tx pgx.Tx) error {
		if err := s.emailVerificationTokenRepo.MarkUsedByUserIDWithTx(ctx, tx, u.ID); err != nil {
			return err
		}
		if err := s.emailVerificationTokenRepo.CreateWithTx(ctx, tx, verificationToken); err != nil {
			return err
		}
		return s.enqueueTokenEmail(ctx, tx, emaildomain.EventTypeVerification, u.Email, plainToken)
	}); err != nil {
		return errx.Internal("token storage failed", err.Error())
	}

	return nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, email string) error {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil
	}

	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return errx.Internal("token generation failed", err.Error())
	}

	resetToken := &authdto.PasswordResetToken{
		UserID:    u.ID,
		TokenHash: tokenHash,
		ExpiresAt: s.clock().Add(s.emailCfg.PasswordResetTokenTTL),
	}

	// Invalidate outstanding reset tokens, persist the new one, and enqueue its
	// email atomically. Anti-enumeration is preserved: unknown emails return
	// nil above before any state change.
	if err := s.withTx(ctx, func(tx pgx.Tx) error {
		if err := s.passwordResetTokenRepo.MarkUsedByUserIDWithTx(ctx, tx, u.ID); err != nil {
			return err
		}
		if err := s.passwordResetTokenRepo.CreateWithTx(ctx, tx, resetToken); err != nil {
			return err
		}
		return s.enqueueTokenEmail(ctx, tx, emaildomain.EventTypePasswordReset, u.Email, plainToken)
	}); err != nil {
		return errx.Internal("token storage failed", err.Error())
	}

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

	if s.clock().After(resetToken.ExpiresAt) {
		return errx.BadRequest("reset token has expired")
	}

	if !passwordHasLetterAndDigit(newPassword) {
		return errx.BadRequest("password must contain at least one letter and one digit")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errx.Internal("password hashing failed", err.Error())
	}

	// Fetch the recipient before the transaction so the password-changed
	// notification can be enqueued atomically with the password change.
	u, err := s.userRepo.GetByID(ctx, resetToken.UserID)
	if err != nil {
		return errx.Internal("user lookup failed", err.Error())
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return errx.Internal("transaction begin failed", err.Error())
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			s.logger.Debug("deferred rollback after commit", zap.Error(err))
		}
	}()

	if err := s.passwordResetTokenRepo.MarkUsedWithTx(ctx, tx, resetToken.ID); err != nil {
		return errx.Internal("token invalidation failed", err.Error())
	}

	if err := s.userRepo.UpdatePasswordWithTx(ctx, tx, resetToken.UserID, string(hash)); err != nil {
		return errx.Internal("password update failed", err.Error())
	}

	if err := s.sessionRepo.RevokeByUserIDWithTx(ctx, tx, resetToken.UserID); err != nil {
		return errx.Internal("session revocation failed", err.Error())
	}

	if err := s.refreshTokenRepo.RevokeByUserIDWithTx(ctx, tx, resetToken.UserID); err != nil {
		return errx.Internal("refresh token revocation failed", err.Error())
	}

	changedEvent := emaildomain.NewNotificationEvent(emaildomain.EventTypePasswordChanged, u.Email)
	if err := s.outboxRepo.EnqueueWithTx(ctx, tx, changedEvent); err != nil {
		return errx.Internal("notification enqueue failed", err.Error())
	}

	if err := tx.Commit(ctx); err != nil {
		return errx.Internal("transaction commit failed", err.Error())
	}

	return nil
}

func (s *AuthService) GetCurrentUser(ctx context.Context, userID string) (*authdto.UserResponse, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, usermodel.ErrUserNotFound) {
			return nil, errx.Unauthorized("user not found")
		}
		return nil, errx.Internal("user lookup failed", err.Error())
	}

	return &authdto.UserResponse{
		ID:              u.ID,
		Email:           u.Email,
		Role:            u.Role,
		IsActive:        u.IsActive,
		EmailVerifiedAt: u.EmailVerifiedAt,
		CreatedAt:       u.CreatedAt,
	}, nil
}

// passwordHasLetterAndDigit reports whether p contains at least one letter and
// one digit — the supplementary password rule shared by Register and
// ResetPassword.
func passwordHasLetterAndDigit(p string) bool {
	hasLetter := false
	hasDigit := false
	for _, c := range p {
		if unicode.IsLetter(c) {
			hasLetter = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

// newRefreshToken generates a refresh token for the session, returning the
// plaintext (for the client) and the *authdto.RefreshToken to persist. It is
// pure: the caller chooses the transactional or non-transactional persistence
// path, keeping transaction boundaries explicit.
func (s *AuthService) newRefreshToken(sessionID string) (string, *authdto.RefreshToken, error) {
	plainToken, tokenHash, err := s.tokenHelper.GenerateRefreshToken()
	if err != nil {
		return "", nil, err
	}
	return plainToken, &authdto.RefreshToken{
		SessionID: sessionID,
		TokenHash: tokenHash,
		ExpiresAt: s.clock().Add(s.tokenHelper.RefreshTTL()),
	}, nil
}

// withTx runs fn inside a database transaction, committing on success and
// rolling back on any error. It centralizes the commit/rollback boilerplate for
// the auth flows that persist state and enqueue an outbox email atomically.
func (s *AuthService) withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.logger.Debug("deferred rollback after commit", zap.Error(err))
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// enqueueTokenEmail enqueues a token-carrying email event within tx.
func (s *AuthService) enqueueTokenEmail(ctx context.Context, tx pgx.Tx, eventType, recipient, token string) error {
	event, err := emaildomain.NewTokenEvent(eventType, recipient, token)
	if err != nil {
		return err
	}
	return s.outboxRepo.EnqueueWithTx(ctx, tx, event)
}

// buildTokenResponse mints an access token and assembles the TokenResponse from
// an already-persisted refresh token's plaintext.
func (s *AuthService) buildTokenResponse(userID, role, sessionID, plainRefreshToken string) (*authdto.TokenResponse, error) {
	accessToken, err := s.tokenHelper.GenerateAccessToken(userID, role, sessionID)
	if err != nil {
		return nil, errx.Internal("access token generation failed", err.Error())
	}
	return &authdto.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: plainRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenHelper.AccessTTLSeconds(),
	}, nil
}
