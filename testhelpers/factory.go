package testhelpers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/ilmannafi/fiber-boilerplate/internal/domain/auth"
	"github.com/ilmannafi/fiber-boilerplate/internal/domain/user"
)

// CreateTestUser inserts a user with email_verified_at set (verified user).
func CreateTestUser(t testing.TB, pool *pgxpool.Pool, email, password string) *user.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	id := uuid.New().String()
	now := time.Now()
	verifiedAt := now

	var u user.User
	err = pool.QueryRow(context.Background(), `
		INSERT INTO users (id, email, password_hash, role, is_active, email_verified_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'user', true, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, id, email, string(hash), verifiedAt, now, now).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	u.Email = email
	u.PasswordHash = ""
	u.Role = user.RoleUser
	u.IsActive = true
	u.EmailVerifiedAt = &verifiedAt

	return &u
}

// CreateVerifiedTestUser inserts a user with email_verified_at set.
// Alias for CreateTestUser which already creates verified users.
func CreateVerifiedTestUser(t testing.TB, pool *pgxpool.Pool, email, password string) *user.User {
	t.Helper()
	return CreateTestUser(t, pool, email, password)
}

// CreateTestSession inserts a session for the given user ID.
func CreateTestSession(t testing.TB, pool *pgxpool.Pool, userID string) *auth.Session {
	t.Helper()

	id := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	var s auth.Session
	err := pool.QueryRow(context.Background(), `
		INSERT INTO sessions (id, user_id, user_agent, ip_address, expires_at, created_at)
		VALUES ($1, $2, 'test', '127.0.0.1', $3, $4)
		RETURNING id, created_at
	`, id, userID, expiresAt, now).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	s.UserID = userID
	s.UserAgent = "test"
	s.IPAddress = "127.0.0.1"
	s.ExpiresAt = expiresAt

	return &s
}

// CreateTestRefreshToken inserts a refresh token for the given session ID.
// Returns the plain text token (for client use) and the token struct.
func CreateTestRefreshToken(t testing.TB, pool *pgxpool.Pool, sessionID string) (string, *auth.RefreshToken) {
	t.Helper()

	// Generate random 32-byte token
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}
	plainToken := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)

	// SHA-256 hash for storage
	h := sha256.Sum256([]byte(plainToken))
	tokenHash := hex.EncodeToString(h[:])

	id := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(168 * time.Hour) // 7 days

	var rt auth.RefreshToken
	err := pool.QueryRow(context.Background(), `
		INSERT INTO refresh_tokens (id, session_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, id, sessionID, tokenHash, expiresAt, now).Scan(&rt.ID, &rt.CreatedAt)
	if err != nil {
		t.Fatalf("failed to create test refresh token: %v", err)
	}

	rt.SessionID = sessionID
	rt.TokenHash = tokenHash
	rt.ExpiresAt = expiresAt

	return plainToken, &rt
}
