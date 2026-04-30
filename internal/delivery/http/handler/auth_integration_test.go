//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	authrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/auth"
	userrepo "github.com/ilmannafi/fiber-boilerplate/internal/repository/user"
	authservice "github.com/ilmannafi/fiber-boilerplate/internal/service/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	authdto "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/delivery/http/middleware"
	"github.com/ilmannafi/fiber-boilerplate/pkg/response"
	"github.com/ilmannafi/fiber-boilerplate/testhelpers"
	"go.uber.org/zap"
)

// captureEmailSender captures email arguments for handler integration tests.
type captureEmailSender struct {
	verificationEmails []struct{ To, Token string }
	resetEmails        []struct{ To, Token string }
}

func (c *captureEmailSender) SendVerificationEmail(_ context.Context, to, token string) error {
	c.verificationEmails = append(c.verificationEmails, struct{ To, Token string }{to, token})
	return nil
}

func (c *captureEmailSender) SendPasswordResetEmail(_ context.Context, to, token string) error {
	c.resetEmails = append(c.resetEmails, struct{ To, Token string }{to, token})
	return nil
}

func (c *captureEmailSender) SendPasswordChangedNotification(_ context.Context, _ string) error {
	return nil
}

type AuthHandlerIntegrationSuite struct {
	suite.Suite
	container   *postgres.PostgresContainer
	pool        *pgxpool.Pool
	app         *fiber.App
	emailSender *captureEmailSender
}

func testHandlerAuthConfig() config.AuthConfig {
	return config.AuthConfig{
		JWTSecret:           "test-secret-key-that-is-long-enough",
		JWTAccessTTL:        15 * time.Minute,
		JWTRefreshTTL:       168 * time.Hour,
		RefreshGracePeriod:  30 * time.Second,
		RegistrationEnabled: true,
	}
}

func (s *AuthHandlerIntegrationSuite) SetupSuite() {
	ctx := context.Background()

	container, pool := testhelpers.StartPostgres(ctx, s.T())
	s.container = container
	s.pool = pool

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(s.T(), err)
	testhelpers.RunMigrations(s.T(), dsn)

	s.setupApp(testHandlerAuthConfig())
}

func (s *AuthHandlerIntegrationSuite) setupApp(cfg config.AuthConfig) {
	uRepo := userrepo.NewUserRepository(s.pool)
	sessRepo := authrepo.NewSessionRepository(s.pool)
	rtRepo := authrepo.NewRefreshTokenRepository(s.pool)
	evTokenRepo := authrepo.NewEmailVerificationTokenRepository(s.pool)
	resetTokenRepo := authrepo.NewPasswordResetTokenRepository(s.pool)

	emailSender := &captureEmailSender{}
	s.emailSender = emailSender

	tokenHelper := authservice.NewTokenHelper(cfg)

	emailCfg := config.EmailConfig{
		VerificationEnabled:   true,
		VerificationTokenTTL:  24 * time.Hour,
		PasswordResetTokenTTL: 15 * time.Minute,
	}

	authSvc := authservice.NewAuthService(
		uRepo, sessRepo, rtRepo,
		evTokenRepo, resetTokenRepo,
		tokenHelper, emailSender,
		cfg, emailCfg,
		zap.NewNop(), s.pool,
	)

	authHandler := NewAuthHandler(authSvc)
	jwtMiddleware := middleware.JWTAuth(tokenHelper)

	app := fiber.New(fiber.Config{
		ErrorHandler: response.ErrorHandler(nil),
	})

	authGroup := app.Group("/api/v1/auth")
	authGroup.Post("/register", authHandler.Register)
	authGroup.Post("/login", authHandler.Login)
	authGroup.Post("/refresh", authHandler.Refresh)
	authGroup.Post("/logout", authHandler.Logout)
	authGroup.Post("/verify-email", authHandler.VerifyEmail)
	authGroup.Post("/forgot-password", authHandler.ForgotPassword)
	authGroup.Post("/reset-password", authHandler.ResetPassword)
	authGroup.Get("/me", jwtMiddleware, authHandler.Me)

	s.app = app
}

func (s *AuthHandlerIntegrationSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		ctx := context.Background()
		_ = s.container.Terminate(ctx)
	}
}

func (s *AuthHandlerIntegrationSuite) SetupTest() {
	testhelpers.TruncateAllTables(s.T(), s.pool)
	s.emailSender = &captureEmailSender{}
	// Rebuild app with fresh email sender
	s.setupApp(testHandlerAuthConfig())
}

func (s *AuthHandlerIntegrationSuite) decodeResponse(body io.Reader) response.Response {
	s.T().Helper()
	var result response.Response
	b, err := io.ReadAll(body)
	require.NoError(s.T(), err)
	require.NoError(s.T(), json.Unmarshal(b, &result))
	return result
}

// Helper: register, verify, login — returns access and refresh tokens
func (s *AuthHandlerIntegrationSuite) registerVerifyLogin(email, password string) (string, string) {
	// Register
	body, _ := json.Marshal(authdto.RegisterRequest{Email: email, Password: password})
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 201, resp.StatusCode)

	// Extract verification token
	time.Sleep(50 * time.Millisecond)
	require.NotEmpty(s.T(), s.emailSender.verificationEmails)
	token := s.emailSender.verificationEmails[0].Token

	// Verify email
	verifyBody, _ := json.Marshal(map[string]string{"token": token})
	req = httptest.NewRequest("POST", "/api/v1/auth/verify-email", bytes.NewReader(verifyBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = s.app.Test(req)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 200, resp.StatusCode)

	// Login
	loginBody, _ := json.Marshal(authdto.LoginRequest{Email: email, Password: password})
	req = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = s.app.Test(req)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 200, resp.StatusCode)

	result := s.decodeResponse(resp.Body)
	dataBytes, err := json.Marshal(result.Data)
	require.NoError(s.T(), err)

	var tokenResp authdto.TokenResponse
	require.NoError(s.T(), json.Unmarshal(dataBytes, &tokenResp))

	return tokenResp.AccessToken, tokenResp.RefreshToken
}

// --- Tests ---

func (s *AuthHandlerIntegrationSuite) TestHTTP_Register_Login_Logout_Flow() {
	accessToken, refreshToken := s.registerVerifyLogin("http-test@example.com", "Password123")
	assert.NotEmpty(s.T(), accessToken)
	assert.NotEmpty(s.T(), refreshToken)

	// Refresh
	refreshBody, _ := json.Marshal(authdto.RefreshRequest{RefreshToken: refreshToken})
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewReader(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 200, resp.StatusCode)

	result := s.decodeResponse(resp.Body)
	dataBytes, _ := json.Marshal(result.Data)
	var newTokens authdto.TokenResponse
	require.NoError(s.T(), json.Unmarshal(dataBytes, &newTokens))
	assert.NotEmpty(s.T(), newTokens.RefreshToken)

	// Logout
	logoutBody, _ := json.Marshal(authdto.LogoutRequest{RefreshToken: newTokens.RefreshToken})
	req = httptest.NewRequest("POST", "/api/v1/auth/logout", bytes.NewReader(logoutBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = s.app.Test(req)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 200, resp.StatusCode)
}

func (s *AuthHandlerIntegrationSuite) TestHTTP_ProtectedRoute_ValidToken() {
	accessToken, _ := s.registerVerifyLogin("protected@example.com", "Password123")

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 200, resp.StatusCode)

	result := s.decodeResponse(resp.Body)
	assert.True(s.T(), result.Success)

	dataBytes, _ := json.Marshal(result.Data)
	var userResp authdto.UserResponse
	require.NoError(s.T(), json.Unmarshal(dataBytes, &userResp))
	assert.Equal(s.T(), "protected@example.com", userResp.Email)
}

func (s *AuthHandlerIntegrationSuite) TestHTTP_ProtectedRoute_MissingToken() {
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 401, resp.StatusCode)
}

func (s *AuthHandlerIntegrationSuite) TestHTTP_ProtectedRoute_InvalidToken() {
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 401, resp.StatusCode)
}

func (s *AuthHandlerIntegrationSuite) TestHTTP_ProtectedRoute_ExpiredToken() {
	// Create a TokenHelper with already-expired access TTL
	expiredCfg := config.AuthConfig{
		JWTSecret:           "test-secret-key-that-is-long-enough",
		JWTAccessTTL:        -1 * time.Second, // already expired
		JWTRefreshTTL:       168 * time.Hour,
		RefreshGracePeriod:  30 * time.Second,
		RegistrationEnabled: true,
	}
	expiredHelper := authservice.NewTokenHelper(expiredCfg)

	// Generate an expired access token manually
	accessToken, _ := expiredHelper.GenerateAccessToken("user-1", "user", "session-1")

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := s.app.Test(req)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), 401, resp.StatusCode)
}

func TestAuthHandlerIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AuthHandlerIntegrationSuite))
}
