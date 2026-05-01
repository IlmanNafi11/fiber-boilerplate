//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	authrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/auth"
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

	svc := NewAuthService(
		uRepo, sessRepo, rtRepo,
		evTokenRepo, resetTokenRepo,
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
	s.emailSender = &CaptureEmailSender{}
	s.svc.emailSender = s.emailSender
}

// --- Helper ---

func (s *AuthIntegrationSuite) registerAndVerify(email, password string) {
	ctx := context.Background()

	_, err := s.svc.Register(ctx, &aservice.RegisterRequest{
		Email:    email,
		Password: password,
	})
	require.NoError(s.T(), err)

	// Wait for email and extract token
	time.Sleep(50 * time.Millisecond)
	emails := s.emailSender.GetVerificationEmails()
	require.NotEmpty(s.T(), emails, "expected verification email to be sent")

	err = s.svc.VerifyEmail(ctx, emails[0].Token)
	require.NoError(s.T(), err)
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

	// Wait for email, extract verification token
	time.Sleep(50 * time.Millisecond)
	emails := s.emailSender.GetVerificationEmails()
	require.NotEmpty(s.T(), emails)

	// Verify email
	err = s.svc.VerifyEmail(ctx, emails[0].Token)
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

	// Verify email
	time.Sleep(50 * time.Millisecond)
	emails := s.emailSender.GetVerificationEmails()
	require.NotEmpty(s.T(), emails)
	err = s.svc.VerifyEmail(ctx, emails[0].Token)
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

	// Extract reset token from email
	time.Sleep(50 * time.Millisecond)
	resetEmails := s.emailSender.GetResetEmails()
	require.NotEmpty(s.T(), resetEmails)

	// Reset password
	err = s.svc.ResetPassword(ctx, resetEmails[0].Token, "NewPassword123")
	require.NoError(s.T(), err)

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

func TestAuthIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AuthIntegrationSuite))
}
