package response

import (
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/ilmannafi/fiber-boilerplate/pkg/errx"
	"go.uber.org/zap"
)

// ErrorHandler returns a fiber.ErrorHandler that maps all error types to
// consistent JSON envelope responses. Internal details are logged via zap
// but never sent to clients.
func ErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		switch e := err.(type) {
		case *errx.AppError:
			if e.Detail != "" {
				logger.Error("app error",
					zap.String("code", e.Code),
					zap.String("detail", e.Detail),
					zap.Int("status", e.HTTPStatus),
				)
			}
			return c.Status(e.HTTPStatus).JSON(Response{
				Success: false,
				Code:    e.Code,
				Message: e.Message,
			})

		case validator.ValidationErrors:
			errItems := make([]ErrorItem, len(e))
			for i, fe := range e {
				errItems[i] = formatValidationError(fe)
			}
			return c.Status(http.StatusUnprocessableEntity).JSON(Response{
				Success: false,
				Code:    "VALIDATION_ERROR",
				Message: "Validation failed",
				Errors:  errItems,
			})

		case *fiber.Error:
			if e.Code >= 500 {
				logger.Error("fiber error",
					zap.Int("status", e.Code),
					zap.String("message", e.Message),
				)
			}
			msg := e.Message
			code := fiberErrorCode(e.Code)
			if e.Code >= 500 {
				msg = "Internal Server Error"
			}
			return c.Status(e.Code).JSON(Response{
				Success: false,
				Code:    code,
				Message: msg,
			})

		default:
			logger.Error("unhandled error",
				zap.String("message", err.Error()),
			)
			return c.Status(http.StatusInternalServerError).JSON(Response{
				Success: false,
				Code:    "INTERNAL_ERROR",
				Message: "Internal Server Error",
			})
		}
	}
}

// fiberErrorCode maps a *fiber.Error HTTP status to a stable SNAKE_CASE machine
// code, mirroring the codes produced by errx constructors. Any 5xx is masked to
// INTERNAL_ERROR so internal failures share one opaque code.
func fiberErrorCode(status int) string {
	if status >= 500 {
		return "INTERNAL_ERROR"
	}
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusUnprocessableEntity:
		return "VALIDATION_ERROR"
	case http.StatusTooManyRequests:
		return "TOO_MANY_REQUESTS"
	default:
		return "ERROR"
	}
}

// validationMsgs maps go-playground/validator tags to human-readable messages.
var validationMsgs = map[string]string{
	"required": "this field is required",
	"email":    "invalid email format",
	"uuid":     "must be a valid UUID",
	"min":      "value is too short",
	"max":      "value is too long",
	"oneof":    "invalid value",
	"len":      "must be exactly {param} characters",
	"gte":      "must be greater than or equal to {param}",
	"lte":      "must be less than or equal to {param}",
	"gt":       "must be greater than {param}",
	"lt":       "must be less than {param}",
	"alphanum": "must contain only letters and numbers",
	"alpha":    "must contain only letters",
	"numeric":  "must be numeric",
	"url":      "must be a valid URL",
	"datetime": "must be a valid datetime",
}

// formatValidationError maps a validator.FieldError to an ErrorItem with a
// human-readable message. Field names come from JSON tags (via RegisterTagNameFunc).
func formatValidationError(fe validator.FieldError) ErrorItem {
	msg, ok := validationMsgs[fe.Tag()]
	if !ok {
		return ErrorItem{
			Field:   fe.Field(),
			Message: fe.Error(),
		}
	}
	// Replace {param} placeholder with the actual parameter value
	msg = strings.ReplaceAll(msg, "{param}", fe.Param())
	return ErrorItem{
		Field:   fe.Field(),
		Message: msg,
	}
}
