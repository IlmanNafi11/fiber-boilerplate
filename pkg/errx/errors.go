// Package errx provides a layered error propagation system for the application.
//
// The error flow follows three layers:
//   - Repository: returns generic sentinel errors (ErrNotFound, ErrDuplicate, etc.)
//     with no HTTP knowledge.
//   - Service: detects sentinel errors via errors.Is and wraps them into AppError
//     with appropriate HTTP status codes and error codes.
//   - Handler: returns the error; the centralized ErrorHandler in pkg/response
//     maps it to a JSON envelope response.
//
// AppError.Detail is internal-only — it is logged via zap but never sent to clients.
package errx

import (
	"errors"
	"fmt"
)

// AppError is the application's standard error type for HTTP-resolvable errors.
// It carries both client-safe (Code, Message) and internal-only (Detail) information.
type AppError struct {
	Code       string // SNAKE_CASE error code (e.g., NOT_FOUND, VALIDATION_ERROR)
	Message    string // Safe-for-client human-readable message
	HTTPStatus int    // HTTP status code (400, 404, 500, etc.)
	Detail     string // Internal-only detail — logged but never sent to client
}

// Error implements the error interface. Returns Code and Message for logging.
func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithDetail chains additional internal detail onto an AppError.
// The detail is logged but never exposed to the client.
func (e *AppError) WithDetail(detail string) *AppError {
	e.Detail = detail
	return e
}

// Sentinel errors — used by the repository layer. No HTTP knowledge.
var (
	ErrNotFound            = errors.New("resource not found")
	ErrDuplicate           = errors.New("duplicate resource")
	ErrConstraintViolation = errors.New("constraint violation")
)

// Constructor functions return *AppError with appropriate HTTP semantics.

func NotFound(message string) *AppError {
	return &AppError{
		Code:       "NOT_FOUND",
		Message:    message,
		HTTPStatus: 404,
	}
}

func Unauthorized(message string) *AppError {
	return &AppError{
		Code:       "UNAUTHORIZED",
		Message:    message,
		HTTPStatus: 401,
	}
}

func Forbidden(message string) *AppError {
	return &AppError{
		Code:       "FORBIDDEN",
		Message:    message,
		HTTPStatus: 403,
	}
}

func BadRequest(message string) *AppError {
	return &AppError{
		Code:       "BAD_REQUEST",
		Message:    message,
		HTTPStatus: 400,
	}
}

func Validation(message string) *AppError {
	return &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    message,
		HTTPStatus: 422,
	}
}

// Internal creates a 500 AppError. The message param is stored in Detail (for logging),
// while the client-facing Message is hardcoded to "Internal Server Error" to prevent
// accidental internal detail leaks.
func Internal(message, detail string) *AppError {
	return &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "Internal Server Error",
		HTTPStatus: 500,
		Detail:     fmt.Sprintf("%s: %s", message, detail),
	}
}

func Conflict(message string) *AppError {
	return &AppError{
		Code:       "CONFLICT",
		Message:    message,
		HTTPStatus: 409,
	}
}

func TooManyRequests(message string) *AppError {
	return &AppError{
		Code:       "TOO_MANY_REQUESTS",
		Message:    message,
		HTTPStatus: 429,
	}
}
