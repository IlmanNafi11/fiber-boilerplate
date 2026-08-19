//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	emaildomain "github.com/ilmannafi/fiber-boilerplate/internal/domain/email"
	authrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/auth"
	outboxrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/emailoutbox"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/ilmannafi/fiber-boilerplate/testhelpers"
	"go.uber.org/zap"
)

type AuthIntegrationSuite struct {
	suite.Suite
	container   *postgres.PostgresContainer
	pool        *pgxpool.Pool
	svc         *AuthService
	emailSender *CaptureEmailSender
}

func (s *AuthIntegrationSuite) SetupSuite() {
	ctx := context.Background()
	container, pool := testhelpers.StartPostgres(ctx, s.T())
	s.container = container
	s.pool = pool

	// Get DSN for migrations
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(s.T(), err)
	testhelpers.RunMigrations(s.T(), dsn)

	s.setupService(testAuthConfig(), testEmailConfig(true))
}

func (s *AuthIntegrationSuite) setupService(cfg config.AuthConfig, emailCfg config.EmailConfig) {
	uRepo := userrepo.NewUserRepository(s.pool)
	sessRepo := authrepo.NewSessionRepository(s.pool)
	rtRepo := authrepo.NewRefreshTokenRepository(s.pool)
	evTokenRepo := authrepo.NewEmailVerificationTokenRepository(s.pool)
	resetTokenRepo := authrepo.NewPasswordResetTokenRepository(s.pool)

	emailSender := &CaptureEmailSender{}
	s.emailSender = emailSender

	tokenHelper := NewTokenHelper(cfg)

	outboxRepo := outboxrepo.NewRepository(s.pool)

	svc := NewAuthService(
		uRepo, sessRepo, rtRepo,
		evTokenRepo, resetTokenRepo,
		outboxRepo,
		tokenHelper, emailSender,
		cfg, emailCfg,
		zap.NewNop(), s.pool,
	)
	s.svc = svc
}

func (s *AuthIntegrationSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		ctx := context.Background()
		if err := s.container.Terminate(ctx); err != nil {
			s.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (s *AuthIntegrationSuite) SetupTest() {
	testhelpers.TruncateAllTables(s.T(), s.pool)
	// Rebuild the default service so tests that swap in a custom config
	// (e.g. registration disabled) do not leak into subsequent tests.
	s.setupService(testAuthConfig(), testEmailConfig(true))
}

// --- Helper ---

func (s *AuthIntegrationSuite) registerAndVerify(email, password string) {
	ctx := context.Background()

	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    email,
		Password: password,
	})
	require.NoError(s.T(), err)

	token := s.outboxToken(emaildomain.EventTypeVerification, email)
	err = s.svc.VerifyEmail(ctx, token)
	require.NoError(s.T(), err)
}

// outboxTokenPayloads returns the decoded token payloads of pending outbox
// events of the given type for a recipient, newest first.
func (s *AuthIntegrationSuite) outboxTokens(eventType, recipient string) []string {
	rows, err := s.pool.Query(context.Background(),
		`SELECT payload FROM email_outbox
		 WHERE event_type = $1 AND recipient = $2
		 ORDER BY created_at DESC`,
		eventType, recipient,
	)
	require.NoError(s.T(), err)
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var payload []byte
		require.NoError(s.T(), rows.Scan(&payload))
		var p emaildomain.TokenPayload
		require.NoError(s.T(), json.Unmarshal(payload, &p))
		tokens = append(tokens, p.Token)
	}
	require.NoError(s.T(), rows.Err())
	return tokens
}

// outboxToken returns the single expected token for a recipient/event type.
func (s *AuthIntegrationSuite) outboxToken(eventType, recipient string) string {
	tokens := s.outboxTokens(eventType, recipient)
	require.NotEmpty(s.T(), tokens, "expected a %s outbox event for %s", eventType, recipient)
	return tokens[0]
}

// outboxCount returns the number of outbox events of a type for a recipient.
func (s *AuthIntegrationSuite) outboxCount(eventType, recipient string) int {
	var n int
	require.NoError(s.T(), s.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM email_outbox WHERE event_type = $1 AND recipient = $2`,
		eventType, recipient,
	).Scan(&n))
	return n
}

// --- Tests ---

func (s *AuthIntegrationSuite) TestRegister_Login_FullFlow() {
	ctx := context.Background()

	// Register
	user, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "test@example.com",
		Password: "Password123",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "test@example.com", user.Email)

	// Verification token is enqueued transactionally to the outbox.
	token := s.outboxToken(emaildomain.EventTypeVerification, "test@example.com")
	err = s.svc.VerifyEmail(ctx, token)
	require.NoError(s.T(), err)

	// Login
	tokenResp, err := s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "test@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), tokenResp.AccessToken)
	assert.NotEmpty(s.T(), tokenResp.RefreshToken)
	assert.Equal(s.T(), "Bearer", tokenResp.TokenType)
	assert.Greater(s.T(), tokenResp.ExpiresIn, 0)

	// Refresh token
	newResp, err := s.svc.RefreshToken(ctx, tokenResp.RefreshToken)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), newResp.AccessToken)
	assert.NotEmpty(s.T(), newResp.RefreshToken)
	assert.NotEqual(s.T(), tokenResp.RefreshToken, newResp.RefreshToken)

	// Logout
	err = s.svc.Logout(ctx, newResp.RefreshToken)
	require.NoError(s.T(), err)

	// Old refresh token should fail after logout (session revoked)
	_, err = s.svc.RefreshToken(ctx, tokenResp.RefreshToken)
	assert.Error(s.T(), err)
}

func (s *AuthIntegrationSuite) TestRegister_EmailVerificationRequired() {
	ctx := context.Background()

	// Service has email verification enabled (default in setupService)
	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "verify-test@example.com",
		Password: "Password123",
	})
	require.NoError(s.T(), err)

	// Try login before verification → should fail
	_, err = s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "verify-test@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.Error(s.T(), err)
	assert.Equal(s.T(), 403, err.(*errx.AppError).HTTPStatus)
	assert.Contains(s.T(), err.(*errx.AppError).Message, "email verification required")

	// Verify email using the outbox token.
	token := s.outboxToken(emaildomain.EventTypeVerification, "verify-test@example.com")
	err = s.svc.VerifyEmail(ctx, token)
	require.NoError(s.T(), err)

	// Login should now succeed
	_, err = s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "verify-test@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.NoError(s.T(), err)
}

func (s *AuthIntegrationSuite) TestRegister_RegistrationDisabled() {
	s.setupService(config.AuthConfig{
		JWTSecret:           "test-secret-key-that-is-long-enough",
		JWTAccessTTL:        15 * time.Minute,
		JWTRefreshTTL:       168 * time.Hour,
		RefreshGracePeriod:  30 * time.Second,
		RegistrationEnabled: false,
	}, testEmailConfig(false))

	ctx := context.Background()
	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "disabled@example.com",
		Password: "Password123",
	})
	require.Error(s.T(), err)
	assert.Equal(s.T(), 403, err.(*errx.AppError).HTTPStatus)
}

func (s *AuthIntegrationSuite) TestRegister_DuplicateEmail() {
	ctx := context.Background()

	// First registration
	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "dup@example.com",
		Password: "Password123",
	})
	require.NoError(s.T(), err)

	// Duplicate → 409
	_, err = s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "dup@example.com",
		Password: "DifferentPassword123",
	})
	require.Error(s.T(), err)
	assert.Equal(s.T(), 409, err.(*errx.AppError).HTTPStatus)
	assert.Contains(s.T(), err.(*errx.AppError).Message, "email already registered")
}

func (s *AuthIntegrationSuite) TestLogin_InvalidCredentials() {
	ctx := context.Background()

	s.registerAndVerify("login-bad@example.com", "Password123")

	// Wrong password
	_, err := s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "login-bad@example.com",
		Password: "WrongPassword123",
	}, "test-agent", "127.0.0.1")
	require.Error(s.T(), err)
	assert.Equal(s.T(), 401, err.(*errx.AppError).HTTPStatus)
}

func (s *AuthIntegrationSuite) TestLogin_DisabledAccount() {
	ctx := context.Background()

	s.registerAndVerify("disabled-acc@example.com", "Password123")

	// Disable user
	_, err := s.pool.Exec(ctx, "UPDATE users SET is_active = false WHERE email = $1", "disabled-acc@example.com")
	require.NoError(s.T(), err)

	// Login → 403
	_, err = s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "disabled-acc@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.Error(s.T(), err)
	assert.Equal(s.T(), 403, err.(*errx.AppError).HTTPStatus)
	assert.Contains(s.T(), err.(*errx.AppError).Message, "account is disabled")
}

func (s *AuthIntegrationSuite) TestRefreshToken_NormalRotation() {
	ctx := context.Background()

	s.registerAndVerify("rotation@example.com", "Password123")

	tokenResp, err := s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "rotation@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.NoError(s.T(), err)

	// First rotation
	newResp1, err := s.svc.RefreshToken(ctx, tokenResp.RefreshToken)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), tokenResp.RefreshToken, newResp1.RefreshToken)

	// Second rotation
	newResp2, err := s.svc.RefreshToken(ctx, newResp1.RefreshToken)
	require.NoError(s.T(), err)
	assert.NotEqual(s.T(), newResp1.RefreshToken, newResp2.RefreshToken)
}

func (s *AuthIntegrationSuite) TestRefreshToken_ReuseDetection() {
	// Use short grace period for this test
	shortGraceCfg := config.AuthConfig{
		JWTSecret:           "test-secret-key-that-is-long-enough",
		JWTAccessTTL:        15 * time.Minute,
		JWTRefreshTTL:       168 * time.Hour,
		RefreshGracePeriod:  100 * time.Millisecond,
		RegistrationEnabled: true,
	}
	s.setupService(shortGraceCfg, testEmailConfig(true))

	ctx := context.Background()
	s.registerAndVerify("reuse@example.com", "Password123")

	tokenResp, err := s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "reuse@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.NoError(s.T(), err)

	// Rotate → old token is revoked with short grace period
	_, err = s.svc.RefreshToken(ctx, tokenResp.RefreshToken)
	require.NoError(s.T(), err)

	// Wait for grace period to expire
	time.Sleep(200 * time.Millisecond)

	// Reuse old token → 401 reuse detected
	_, err = s.svc.RefreshToken(ctx, tokenResp.RefreshToken)
	require.Error(s.T(), err)
	assert.Equal(s.T(), 401, err.(*errx.AppError).HTTPStatus)
	assert.Contains(s.T(), err.(*errx.AppError).Message, "reuse detected")
}

func (s *AuthIntegrationSuite) TestForgotPassword_ResetPassword_Flow() {
	ctx := context.Background()

	s.registerAndVerify("reset-flow@example.com", "Password123")

	// Forgot password
	err := s.svc.ForgotPassword(ctx, "reset-flow@example.com")
	require.NoError(s.T(), err)

	// Reset token is enqueued to the outbox.
	resetToken := s.outboxToken(emaildomain.EventTypePasswordReset, "reset-flow@example.com")

	// Reset password
	err = s.svc.ResetPassword(ctx, resetToken, "NewPassword123")
	require.NoError(s.T(), err)

	// A password-changed notification is enqueued in the same commit.
	assert.Equal(s.T(), 1, s.outboxCount(emaildomain.EventTypePasswordChanged, "reset-flow@example.com"))

	// Login with old password → fail
	_, err = s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "reset-flow@example.com",
		Password: "Password123",
	}, "test-agent", "127.0.0.1")
	require.Error(s.T(), err)

	// Login with new password → success
	_, err = s.svc.Login(ctx, &aservice.LoginRequest{
		Email:    "reset-flow@example.com",
		Password: "NewPassword123",
	}, "test-agent", "127.0.0.1")
	require.NoError(s.T(), err)
}

func (s *AuthIntegrationSuite) TestGetCurrentUser_Success() {
	ctx := context.Background()

	s.registerAndVerify("me@example.com", "Password123")

	// Get user ID from DB
	var userID string
	err := s.pool.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", "me@example.com").Scan(&userID)
	require.NoError(s.T(), err)

	resp, err := s.svc.GetCurrentUser(ctx, userID)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "me@example.com", resp.Email)
	assert.Equal(s.T(), "user", resp.Role)
	assert.True(s.T(), resp.IsActive)
}

func (s *AuthIntegrationSuite) TestGetCurrentUser_NotFound() {
	ctx := context.Background()

	_, err := s.svc.GetCurrentUser(ctx, "00000000-0000-0000-0000-000000000000")
	require.Error(s.T(), err)
	assert.Equal(s.T(), 401, err.(*errx.AppError).HTTPStatus)
}

// --- Task 5: transactional outbox enqueue ---

func (s *AuthIntegrationSuite) TestRegister_EnqueuesVerificationEventAtomically() {
	ctx := context.Background()

	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "outbox-register@example.com",
		Password: "Password123",
	})
	require.NoError(s.T(), err)

	// Exactly one verification event, carrying a usable token, is persisted —
	// no fire-and-forget goroutine, so the sender is never invoked directly.
	require.Equal(s.T(), 1, s.outboxCount(emaildomain.EventTypeVerification, "outbox-register@example.com"))
	token := s.outboxToken(emaildomain.EventTypeVerification, "outbox-register@example.com")
	require.NotEmpty(s.T(), token)
	assert.Empty(s.T(), s.emailSender.GetVerificationEmails(), "email must be enqueued, not sent inline")

	// The enqueued token verifies the account, proving it committed with the token row.
	require.NoError(s.T(), s.svc.VerifyEmail(ctx, token))
}

func (s *AuthIntegrationSuite) TestForgotPassword_UnknownEmailEnqueuesNothing() {
	err := s.svc.ForgotPassword(context.Background(), "nobody@example.com")
	require.NoError(s.T(), err) // anti-enumeration: no error leak

	assert.Equal(s.T(), 0, s.outboxCount(emaildomain.EventTypePasswordReset, "nobody@example.com"))
}

func (s *AuthIntegrationSuite) TestResendVerification_EnqueuesFreshEvent() {
	ctx := context.Background()

	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    "resend@example.com",
		Password: "Password123",
	})
	require.NoError(s.T(), err)

	require.NoError(s.T(), s.svc.ResendVerification(ctx, "resend@example.com"))

	// Register + resend each enqueue one verification event for the recipient.
	assert.Equal(s.T(), 2, s.outboxCount(emaildomain.EventTypeVerification, "resend@example.com"))

	// The newest token still verifies the account.
	token := s.outboxToken(emaildomain.EventTypeVerification, "resend@example.com")
	require.NoError(s.T(), s.svc.VerifyEmail(ctx, token))
}

func TestAuthIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AuthIntegrationSuite))
}
