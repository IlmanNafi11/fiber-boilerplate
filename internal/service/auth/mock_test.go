package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	aservice "github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
)

// --- Mock Repositories ---

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

func (m *mockRefreshTokenRepo) RevokeByUserIDWithTx(_ context.Context, _ pgx.Tx, _ string) error {
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

func (m *mockSessionRepo) RevokeByUserIDWithTx(_ context.Context, _ pgx.Tx, _ string) error {
	return nil
}

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

func (m *mockPasswordResetTokenRepo) MarkUsedWithTx(_ context.Context, _ pgx.Tx, id string) error {
	return m.Called(id).Error(0)
}

func (m *mockPasswordResetTokenRepo) MarkUsedByUserID(_ context.Context, userID string) error {
	return m.Called(userID).Error(0)
}

// --- CaptureEmailSender captures email arguments for integration tests ---

type emailCapture struct {
	To    string
	Token string
}

type CaptureEmailSender struct {
	mu                 sync.Mutex
	VerificationEmails []emailCapture
	ResetEmails        []emailCapture
	ChangedEmails      []string
}

func (c *CaptureEmailSender) SendVerificationEmail(_ context.Context, to, token string) error {
	c.mu.Lock()
	c.VerificationEmails = append(c.VerificationEmails, emailCapture{To: to, Token: token})
	c.mu.Unlock()
	return nil
}

func (c *CaptureEmailSender) SendPasswordResetEmail(_ context.Context, to, token string) error {
	c.mu.Lock()
	c.ResetEmails = append(c.ResetEmails, emailCapture{To: to, Token: token})
	c.mu.Unlock()
	return nil
}

func (c *CaptureEmailSender) SendPasswordChangedNotification(_ context.Context, to string) error {
	c.mu.Lock()
	c.ChangedEmails = append(c.ChangedEmails, to)
	c.mu.Unlock()
	return nil
}

func (c *CaptureEmailSender) GetVerificationEmails() []emailCapture {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]emailCapture, len(c.VerificationEmails))
	copy(result, c.VerificationEmails)
	return result
}

func (c *CaptureEmailSender) GetResetEmails() []emailCapture {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]emailCapture, len(c.ResetEmails))
	copy(result, c.ResetEmails)
	return result
}

func (c *CaptureEmailSender) GetChangedEmails() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.ChangedEmails))
	copy(result, c.ChangedEmails)
	return result
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
