package auth

import (
	"context"
	"testing"
	"time"

	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

// mockRefreshTokenRepo mocks RefreshTokenRepo for Logout tests.
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

// mockSessionRepo mocks SessionRepo for Logout tests.
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

// --- Tests ---

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
	// sessionRepo.GetByID must NOT be called — short-circuited by token check
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
	// RevokeByID must NOT be called — short-circuited by session check
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
