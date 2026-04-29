package seeder

import (
	"context"
	"testing"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- validatePassword tests ---

func TestValidatePassword_Valid(t *testing.T) {
	err := validatePassword("changeme123")
	assert.NoError(t, err)
}

func TestValidatePassword_TooShort(t *testing.T) {
	err := validatePassword("Ab1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 8 characters")
}

func TestValidatePassword_NoLetter(t *testing.T) {
	err := validatePassword("12345678")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 letter")
}

func TestValidatePassword_NoDigit(t *testing.T) {
	err := validatePassword("abcdefgh")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 digit")
}

func TestValidatePassword_ExactlyEightCharsWithLetterAndDigit(t *testing.T) {
	err := validatePassword("abcd1234")
	assert.NoError(t, err)
}

// --- hashPassword tests ---

func TestHashPassword_Success(t *testing.T) {
	hash, err := hashPassword("changeme123")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "changeme123", hash)
}

func TestHashPassword_DifferentHashesForSameInput(t *testing.T) {
	hash1, err := hashPassword("changeme123")
	require.NoError(t, err)
	hash2, err := hashPassword("changeme123")
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2, "bcrypt should produce different hashes due to random salt")
}

// --- Run() production guard tests ---
// These test the early-return paths before pool.Begin() is called,
// so passing nil pool is safe since execution never reaches the DB call.

func TestRun_ProductionGuardWithoutForce(t *testing.T) {
	cfg := config.SeederConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "changeme123",
	}
	err := Run(context.TODO(), nil, cfg, "production", false, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "seeder refused to run in production environment")
	assert.Contains(t, err.Error(), "--force")
}

func TestRun_ProductionGuardWithForce(t *testing.T) {
	// With force=true, production guard passes but pool.Begin(nil) will panic.
	// We cannot test past this point without a real DB.
	// Instead verify the guard does NOT trigger the error.
	// This test verifies that production+force bypasses the guard;
	// the subsequent nil pointer panic is caught by recover.
	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic from nil pool after production guard passes, got nil")
	}()

	cfg := config.SeederConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "changeme123",
	}
	_ = Run(context.TODO(), nil, cfg, "production", true, zap.NewNop())
}

func TestRun_EmptyAdminEmail(t *testing.T) {
	cfg := config.SeederConfig{
		AdminEmail:    "",
		AdminPassword: "changeme123",
	}
	err := Run(context.TODO(), nil, cfg, "development", false, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_EMAIL and ADMIN_PASSWORD environment variables are required")
}

func TestRun_EmptyAdminPassword(t *testing.T) {
	cfg := config.SeederConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "",
	}
	err := Run(context.TODO(), nil, cfg, "development", false, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_EMAIL and ADMIN_PASSWORD environment variables are required")
}

func TestRun_InvalidAdminPassword(t *testing.T) {
	cfg := config.SeederConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "short",
	}
	err := Run(context.TODO(), nil, cfg, "development", false, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_PASSWORD invalid")
}

// --- seedDemoUsers email validation tests ---
// seedDemoUsers validates DEMO_EMAIL format BEFORE any DB operations,
// so passing nil tx is safe — execution returns before tx is used.

func TestSeedDemoUsers_InvalidEmail_NoAtSign(t *testing.T) {
	cfg := config.SeederConfig{
		DemoEmail:    "notanemail",
		DemoPassword: "changeme123",
	}
	_, _, _, err := seedDemoUsers(context.TODO(), nil, cfg, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid email address")
}

func TestSeedDemoUsers_InvalidEmail_AtSignAtStart(t *testing.T) {
	cfg := config.SeederConfig{
		DemoEmail:    "@example.com",
		DemoPassword: "changeme123",
	}
	_, _, _, err := seedDemoUsers(context.TODO(), nil, cfg, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid email address")
}

func TestSeedDemoUsers_InvalidEmail_AtSignAtEnd(t *testing.T) {
	cfg := config.SeederConfig{
		DemoEmail:    "user@",
		DemoPassword: "changeme123",
	}
	_, _, _, err := seedDemoUsers(context.TODO(), nil, cfg, zap.NewNop())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid email address")
}
