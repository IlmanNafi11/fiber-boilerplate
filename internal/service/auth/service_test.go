package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Logout Tests ---

func TestLogout_ReturnsUnauthorizedForUnknownToken(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, errx.ErrNotFound)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	err := svc.Logout(context.Background(), "nonexistent-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Equal(t, "invalid refresh token", err.(*errx.AppError).Message)

	mockRT.AssertExpectations(t)
	mockSess.AssertNotCalled(t, "GetByID")
}

func TestLogout_ReturnsUnauthorizedForRevokedRefreshToken(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	revokedAt := time.Now().Add(-1 * time.Hour)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			RevokedAt: &revokedAt,
		}, nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	err := svc.Logout(context.Background(), "some-refresh-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Equal(t, "invalid refresh token", err.(*errx.AppError).Message)

	mockRT.AssertExpectations(t)
	mockSess.AssertNotCalled(t, "GetByID")
}

func TestLogout_ReturnsUnauthorizedForRevokedSession(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	revokedAt := time.Now().Add(-1 * time.Hour)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			RevokedAt: nil,
		}, nil)

	mockSess.On("GetByID", mock.Anything, "session-1").
		Return(&aservice.Session{
			ID:        "session-1",
			RevokedAt: &revokedAt,
		}, nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	err := svc.Logout(context.Background(), "some-refresh-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Equal(t, "invalid refresh token", err.(*errx.AppError).Message)

	mockRT.AssertExpectations(t)
	mockSess.AssertExpectations(t)
	mockSess.AssertNotCalled(t, "RevokeByID")
}

func TestLogout_SuccessRevokesActiveSession(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			RevokedAt: nil,
		}, nil)

	mockSess.On("GetByID", mock.Anything, "session-1").
		Return(&aservice.Session{
			ID:        "session-1",
			RevokedAt: nil,
		}, nil)

	mockSess.On("RevokeByID", mock.Anything, "session-1").
		Return(nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	err := svc.Logout(context.Background(), "some-refresh-token")
	require.NoError(t, err)

	mockRT.AssertExpectations(t)
	mockSess.AssertExpectations(t)
}

// --- VerifyEmail Tests ---

func TestVerifyEmail_InvalidToken(t *testing.T) {
	mockEvTokenRepo := new(mockEmailVerificationTokenRepo)

	token := "nonexistent-token"
	tokenHash := sha256Hex(token)

	mockEvTokenRepo.On("GetByTokenHash", tokenHash).Return(nil, errx.ErrNotFound)

	svc := &AuthService{
		emailVerificationTokenRepo: mockEvTokenRepo,
		tokenHelper:                NewTokenHelper(testAuthConfig()),
		logger:                     zap.NewNop(),
	}

	err := svc.VerifyEmail(context.Background(), token)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "invalid or expired")
}

func TestVerifyEmail_ExpiredToken(t *testing.T) {
	mockEvTokenRepo := new(mockEmailVerificationTokenRepo)

	token := "expired-token"
	tokenHash := sha256Hex(token)

	expiredAt := time.Now().Add(-1 * time.Hour)
	mockEvTokenRepo.On("GetByTokenHash", tokenHash).Return(&aservice.EmailVerificationToken{
		ID:        "ev-token-2",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: expiredAt,
		UsedAt:    nil,
	}, nil)

	svc := &AuthService{
		emailVerificationTokenRepo: mockEvTokenRepo,
		tokenHelper:                NewTokenHelper(testAuthConfig()),
		logger:                     zap.NewNop(),
	}

	err := svc.VerifyEmail(context.Background(), token)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "expired")
}

func TestVerifyEmail_UsedToken(t *testing.T) {
	mockEvTokenRepo := new(mockEmailVerificationTokenRepo)

	token := "used-token"
	tokenHash := sha256Hex(token)

	usedAt := time.Now().Add(-1 * time.Hour)
	mockEvTokenRepo.On("GetByTokenHash", tokenHash).Return(&aservice.EmailVerificationToken{
		ID:        "ev-token-3",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		UsedAt:    &usedAt,
	}, nil)

	svc := &AuthService{
		emailVerificationTokenRepo: mockEvTokenRepo,
		tokenHelper:                NewTokenHelper(testAuthConfig()),
		logger:                     zap.NewNop(),
	}

	err := svc.VerifyEmail(context.Background(), token)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "already used")
}

func TestVerifyEmail_TokenLookupInternalError(t *testing.T) {
	mockEvTokenRepo := new(mockEmailVerificationTokenRepo)

	rawInput := "db-error-token"
	tokenHash := sha256Hex(rawInput)

	mockEvTokenRepo.On("GetByTokenHash", tokenHash).Return(nil, errors.New("db connection lost"))

	svc := &AuthService{
		emailVerificationTokenRepo: mockEvTokenRepo,
		tokenHelper:                NewTokenHelper(testAuthConfig()),
		logger:                     zap.NewNop(),
	}

	err := svc.VerifyEmail(context.Background(), rawInput)
	require.Error(t, err)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
}

// --- ResetPassword Tests ---

func TestResetPassword_InvalidToken(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	token := "invalid-token"
	tokenHash := sha256Hex(token)

	mockResetRepo.On("GetByTokenHash", tokenHash).Return(nil, errx.ErrNotFound)

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), token, "NewPassword123")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	token := "expired-reset-token"
	tokenHash := sha256Hex(token)

	expiredAt := time.Now().Add(-1 * time.Minute)
	mockResetRepo.On("GetByTokenHash", tokenHash).Return(&aservice.PasswordResetToken{
		ID:        "reset-2",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: expiredAt,
		UsedAt:    nil,
	}, nil)

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), token, "NewPassword123")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "expired")
}

func TestResetPassword_UsedToken(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	token := "used-reset-token"
	tokenHash := sha256Hex(token)

	usedAt := time.Now().Add(-1 * time.Hour)
	mockResetRepo.On("GetByTokenHash", tokenHash).Return(&aservice.PasswordResetToken{
		ID:        "reset-3",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		UsedAt:    &usedAt,
	}, nil)

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), token, "NewPassword123")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "already used")
}

func TestResetPassword_InvalidNewPassword_NoLetter(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	rawInput := "valid-reset-token"
	tokenHash := sha256Hex(rawInput)

	mockResetRepo.On("GetByTokenHash", tokenHash).Return(&aservice.PasswordResetToken{
		ID:        "reset-4",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		UsedAt:    nil,
	}, nil)

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), rawInput, "12345678")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

func TestResetPassword_InvalidNewPassword_NoDigit(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	rawInput := "valid-reset-token"
	tokenHash := sha256Hex(rawInput)

	mockResetRepo.On("GetByTokenHash", tokenHash).Return(&aservice.PasswordResetToken{
		ID:        "reset-5",
		UserID:    "user-1",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		UsedAt:    nil,
	}, nil)

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), rawInput, "allletters")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

func TestResetPassword_TokenLookupInternalError(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	rawInput := "db-error-token"
	tokenHash := sha256Hex(rawInput)

	mockResetRepo.On("GetByTokenHash", tokenHash).Return(nil, errors.New("db connection lost"))

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), rawInput, "NewPassword123")
	require.Error(t, err)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
}

// --- Register Unit Tests ---

func TestRegister_DisabledReturnsForbidden(t *testing.T) {
	svc := &AuthService{
		cfg:         testAuthConfig(),
		tokenHelper: NewTokenHelper(testAuthConfig()),
		logger:      zap.NewNop(),
	}
	// Override registration enabled
	svc.cfg.RegistrationEnabled = false

	_, err := svc.Register(context.Background(), &aservice.RegisterRequest{
		Email:    "test@example.com",
		Password: "Password123",
	})
	require.Error(t, err)
	assert.Equal(t, 403, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "registration is currently disabled")
}

func TestPasswordHasLetterAndDigit(t *testing.T) {
	cases := []struct {
		name     string
		password string
		want     bool
	}{
		{"letter and digit", "Password123", true},
		{"digits only", "12345678", false},
		{"letters only", "allletters", false},
		{"empty", "", false},
		{"symbol with letter and digit", "a1!", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, passwordHasLetterAndDigit(tc.password))
		})
	}
}

func TestRegister_PasswordNoLetter(t *testing.T) {
	svc := &AuthService{
		cfg:         testAuthConfig(),
		tokenHelper: NewTokenHelper(testAuthConfig()),
		logger:      zap.NewNop(),
		emailCfg:    testEmailConfig(false),
	}

	_, err := svc.Register(context.Background(), &aservice.RegisterRequest{
		Email:    "test@example.com",
		Password: "12345678",
	})
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

func TestRegister_PasswordNoDigit(t *testing.T) {
	svc := &AuthService{
		cfg:         testAuthConfig(),
		tokenHelper: NewTokenHelper(testAuthConfig()),
		logger:      zap.NewNop(),
		emailCfg:    testEmailConfig(false),
	}

	_, err := svc.Register(context.Background(), &aservice.RegisterRequest{
		Email:    "test@example.com",
		Password: "allletters",
	})
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

// --- RefreshToken Unit Tests ---

func TestRefreshToken_TokenNotFound(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, errx.ErrNotFound)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	_, err := svc.RefreshToken(context.Background(), "some-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	mockRT.AssertExpectations(t)
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}, nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	_, err := svc.RefreshToken(context.Background(), "some-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "expired")
	mockRT.AssertExpectations(t)
}

func TestRefreshToken_SessionNotFound(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, nil)

	mockSess.On("GetByID", mock.Anything, "session-1").
		Return(nil, errx.ErrNotFound)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	_, err := svc.RefreshToken(context.Background(), "some-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	mockRT.AssertExpectations(t)
	mockSess.AssertExpectations(t)
}

func TestRefreshToken_RevokedSession(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	revokedAt := time.Now().Add(-1 * time.Hour)

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:        "token-1",
			SessionID: "session-1",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, nil)

	mockSess.On("GetByID", mock.Anything, "session-1").
		Return(&aservice.Session{
			ID:        "session-1",
			RevokedAt: &revokedAt,
		}, nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
	}

	_, err := svc.RefreshToken(context.Background(), "some-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "revoked")
	mockRT.AssertExpectations(t)
	mockSess.AssertExpectations(t)
}

func TestRefreshToken_ReuseOutsideGracePeriod(t *testing.T) {
	mockRT := new(mockRefreshTokenRepo)
	mockSess := new(mockSessionRepo)

	revokedAt := time.Now().Add(-2 * time.Minute)
	graceUntil := time.Now().Add(-1 * time.Minute) // grace period already passed

	mockRT.On("GetByTokenHash", mock.Anything, mock.AnythingOfType("string")).
		Return(&aservice.RefreshToken{
			ID:         "token-1",
			SessionID:  "session-1",
			RevokedAt:  &revokedAt,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			GraceUntil: &graceUntil,
		}, nil)

	mockSess.On("GetByID", mock.Anything, "session-1").
		Return(&aservice.Session{ID: "session-1"}, nil)

	mockSess.On("RevokeByID", mock.Anything, "session-1").Return(nil)
	mockRT.On("RevokeBySessionID", mock.Anything, "session-1").Return(nil)

	svc := &AuthService{
		refreshTokenRepo: mockRT,
		sessionRepo:      mockSess,
		tokenHelper:      NewTokenHelper(testAuthConfig()),
		logger:           zap.NewNop(),
	}

	_, err := svc.RefreshToken(context.Background(), "some-token")
	require.Error(t, err)
	assert.Equal(t, 401, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "reuse detected")
	mockRT.AssertExpectations(t)
	mockSess.AssertExpectations(t)
}

// NOTE: Login, Register (success path), GetCurrentUser, ResendVerification,
// ForgotPassword, RefreshToken (normal rotation + grace rotation) require
// concrete *userrepo.UserRepository and are tested in integration tests
// (see 10-03-PLAN.md).
