package errx

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppError_Error(t *testing.T) {
	err := NotFound("user not found")
	result := err.Error()
	assert.Contains(t, result, "NOT_FOUND")
	assert.Contains(t, result, "user not found")
}

func TestAppError_WithDetail(t *testing.T) {
	err := BadRequest("invalid input").WithDetail("field X failed constraint Y")
	assert.Equal(t, "field X failed constraint Y", err.Detail)
	assert.Equal(t, "invalid input", err.Message)
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotFound", ErrNotFound, "resource not found"},
		{"ErrDuplicate", ErrDuplicate, "duplicate resource"},
		{"ErrConstraintViolation", ErrConstraintViolation, "constraint violation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.msg, tt.err.Error())
			// Sentinel errors must be detectable via errors.Is
			assert.True(t, errors.Is(tt.err, tt.err))
		})
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name   string
		fn     func() *AppError
		code   string
		status int
		msg    string
	}{
		{
			"NotFound", func() *AppError { return NotFound("not here") },
			"NOT_FOUND", 404, "not here",
		},
		{
			"Unauthorized", func() *AppError { return Unauthorized("no auth") },
			"UNAUTHORIZED", 401, "no auth",
		},
		{
			"Forbidden", func() *AppError { return Forbidden("no access") },
			"FORBIDDEN", 403, "no access",
		},
		{
			"BadRequest", func() *AppError { return BadRequest("bad") },
			"BAD_REQUEST", 400, "bad",
		},
		{
			"Validation", func() *AppError { return Validation("val failed") },
			"VALIDATION_ERROR", 422, "val failed",
		},
		{
			"Conflict", func() *AppError { return Conflict("dup") },
			"CONFLICT", 409, "dup",
		},
		{
			"TooManyRequests", func() *AppError { return TooManyRequests("slow down") },
			"TOO_MANY_REQUESTS", 429, "slow down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			assert.Equal(t, tt.code, err.Code)
			assert.Equal(t, tt.status, err.HTTPStatus)
			assert.Equal(t, tt.msg, err.Message)
		})
	}
}

func TestInternal_NeverLeaksDetail(t *testing.T) {
	err := Internal("db query failed", "connection refused")

	// Client-facing message must always be generic
	assert.Equal(t, "Internal Server Error", err.Message)
	assert.Equal(t, "INTERNAL_ERROR", err.Code)
	assert.Equal(t, 500, err.HTTPStatus)

	// Detail is internal-only — must contain both params
	require.NotEmpty(t, err.Detail)
	assert.Contains(t, err.Detail, "db query failed")
	assert.Contains(t, err.Detail, "connection refused")

	// Detail must NOT appear in Error() output (which could be logged alongside)
	// Error() returns Code + Message only
	errStr := err.Error()
	assert.True(t, strings.Contains(errStr, "INTERNAL_ERROR"))
	assert.False(t, strings.Contains(errStr, "connection refused"))
}

func TestAppError_ImplementsError(t *testing.T) {
	var err error = BadRequest("test")
	assert.NotNil(t, err)
	// Must be usable with errors.As
	var appErr *AppError
	assert.True(t, errors.As(err, &appErr))
	assert.Equal(t, "BAD_REQUEST", appErr.Code)
}
