package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Existing Mocks ---

type mockRefreshTokenRepo struct {
	mock.Mock
}

func (m *mockRefreshTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*aservice.RefreshToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*aservice.RefreshToken), args.Error(1)
}

func (m *mockRefreshTokenRepo) Create(ctx context.Context, t *aservice.RefreshToken) error {
	return m.Called(ctx, t).Error(0)
}

func (m *mockRefreshTokenRepo) RevokeBySessionID(ctx context.Context, sessionID string) error {
	return m.Called(ctx, sessionID).Error(0)
}

func (m *mockRefreshTokenRepo) RevokeByUserID(_ context.Context, _ string) error {
	return nil
}

func (m *mockRefreshTokenRepo) RevokeWithTx(_ context.Context, _ pgx.Tx, _ string, _ *time.Time) error {
	return nil
}

func (m *mockRefreshTokenRepo) CreateWithTx(_ context.Context, _ pgx.Tx, _ *aservice.RefreshToken) error {
	return nil
}

type mockSessionRepo struct {
	mock.Mock
}

func (m *mockSessionRepo) Create(ctx context.Context, s *aservice.Session) error {
	return m.Called(ctx, s).Error(0)
}

func (m *mockSessionRepo) GetByID(ctx context.Context, id string) (*aservice.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*aservice.Session), args.Error(1)
}

func (m *mockSessionRepo) RevokeByID(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *mockSessionRepo) RevokeByUserID(_ context.Context, _ string) error {
	return nil
}

// --- Phase 6 Mocks ---

type mockEmailVerificationTokenRepo struct {
	mock.Mock
}

func (m *mockEmailVerificationTokenRepo) Create(_ context.Context, t *aservice.EmailVerificationToken) error {
	t.ID = uuid.New().String()
	return m.Called(t).Error(0)
}

func (m *mockEmailVerificationTokenRepo) GetByTokenHash(_ context.Context, tokenHash string) (*aservice.EmailVerificationToken, error) {
	args := m.Called(tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*aservice.EmailVerificationToken), args.Error(1)
}

func (m *mockEmailVerificationTokenRepo) GetActiveByUserID(_ context.Context, userID string) (*aservice.EmailVerificationToken, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*aservice.EmailVerificationToken), args.Error(1)
}

func (m *mockEmailVerificationTokenRepo) MarkUsed(_ context.Context, id string) error {
	return m.Called(id).Error(0)
}

func (m *mockEmailVerificationTokenRepo) MarkUsedByUserID(_ context.Context, userID string) error {
	return m.Called(userID).Error(0)
}

type mockPasswordResetTokenRepo struct {
	mock.Mock
}

func (m *mockPasswordResetTokenRepo) Create(_ context.Context, t *aservice.PasswordResetToken) error {
	t.ID = uuid.New().String()
	return m.Called(t).Error(0)
}

func (m *mockPasswordResetTokenRepo) GetByTokenHash(_ context.Context, tokenHash string) (*aservice.PasswordResetToken, error) {
	args := m.Called(tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*aservice.PasswordResetToken), args.Error(1)
}

func (m *mockPasswordResetTokenRepo) MarkUsed(_ context.Context, id string) error {
	return m.Called(id).Error(0)
}

func (m *mockPasswordResetTokenRepo) MarkUsedByUserID(_ context.Context, userID string) error {
	return m.Called(userID).Error(0)
}

type mockEmailSender struct {
	sent []string
	err  error
}

func (m *mockEmailSender) SendVerificationEmail(_ context.Context, _, _ string) error {
	m.sent = append(m.sent, "verification")
	return m.err
}

func (m *mockEmailSender) SendPasswordResetEmail(_ context.Context, _, _ string) error {
	m.sent = append(m.sent, "reset")
	return m.err
}

func (m *mockEmailSender) SendPasswordChangedNotification(_ context.Context, _ string) error {
	m.sent = append(m.sent, "changed")
	return m.err
}

// --- Helpers ---

func testEmailConfig(enabled bool) config.EmailConfig {
	return config.EmailConfig{
		VerificationEnabled:   enabled,
		VerificationTokenTTL:  24 * time.Hour,
		PasswordResetTokenTTL: 15 * time.Minute,
	}
}

func sha256Hex(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// --- Existing Logout Tests ---

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

// --- Phase 6 Tests ---

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

	token := "db-error-token"
	tokenHash := sha256Hex(token)

	mockEvTokenRepo.On("GetByTokenHash", tokenHash).Return(nil, errors.New("db connection lost"))

	svc := &AuthService{
		emailVerificationTokenRepo: mockEvTokenRepo,
		tokenHelper:                NewTokenHelper(testAuthConfig()),
		logger:                     zap.NewNop(),
	}

	err := svc.VerifyEmail(context.Background(), token)
	require.Error(t, err)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
}

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

	token := "valid-reset-token"
	tokenHash := sha256Hex(token)

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

	err := svc.ResetPassword(context.Background(), token, "12345678")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

func TestResetPassword_InvalidNewPassword_NoDigit(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	token := "valid-reset-token"
	tokenHash := sha256Hex(token)

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

	err := svc.ResetPassword(context.Background(), token, "allletters")
	require.Error(t, err)
	assert.Equal(t, 400, err.(*errx.AppError).HTTPStatus)
	assert.Contains(t, err.(*errx.AppError).Message, "letter and one digit")
}

func TestResetPassword_TokenLookupInternalError(t *testing.T) {
	mockResetRepo := new(mockPasswordResetTokenRepo)

	token := "db-error-token"
	tokenHash := sha256Hex(token)

	mockResetRepo.On("GetByTokenHash", tokenHash).Return(nil, errors.New("db connection lost"))

	svc := &AuthService{
		passwordResetTokenRepo: mockResetRepo,
		tokenHelper:            NewTokenHelper(testAuthConfig()),
		logger:                 zap.NewNop(),
	}

	err := svc.ResetPassword(context.Background(), token, "NewPassword123")
	require.Error(t, err)
	assert.Equal(t, 500, err.(*errx.AppError).HTTPStatus)
}
