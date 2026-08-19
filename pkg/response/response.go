// Package response provides the unified API response envelope and helper functions.
//
// Every HTTP response follows the same envelope structure:
//   - Success:   {"success": true, "message": "...", "data": {...}}
//   - Error:     {"success": false, "code": "NOT_FOUND", "message": "..."}
//   - Validation:{"success": false, "code": "VALIDATION_ERROR", "message": "...", "errors": [...]}
//   - Paginated: {"success": true, "message": "...", "data": [...], "meta": {...}}
//
// Handlers use the success helpers (OK, Created, etc.) for success responses.
// Errors are returned and handled by the centralized ErrorHandler.
package response

import (
	"github.com/gofiber/fiber/v3"
)

// Response is the unified API response envelope. Fields use omitempty so that
// success responses omit "code"/"errors", error responses omit "data"/"meta",
// and non-paginated responses omit "meta".
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Code    string      `json:"code,omitempty"`
	Data    any         `json:"data,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
	Errors  []ErrorItem `json:"errors,omitempty"`
}

// Meta holds pagination metadata. Only present for paginated responses.
type Meta struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

// ErrorItem represents a single field-level validation error. It is only used
// for validation responses; general errors carry their machine code in
// Response.Code and their human message in Response.Message.
type ErrorItem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// OK sends a 200 response with the success envelope.
func OK(c fiber.Ctx, message string, data any) error {
	return c.Status(fiber.StatusOK).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Created sends a 201 response with the success envelope.
func Created(c fiber.Ctx, message string, data any) error {
	return c.Status(fiber.StatusCreated).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// Accepted sends a 202 response with the success envelope.
func Accepted(c fiber.Ctx, message string, data any) error {
	return c.Status(fiber.StatusAccepted).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// NoContent sends a 204 status with no body (true HTTP 204 No Content).
func NoContent(c fiber.Ctx) error {
	return c.Status(fiber.StatusNoContent).Send(nil)
}

// Paginated sends a 200 response with the success envelope and pagination metadata.
func Paginated(c fiber.Ctx, message string, data any, page, limit, total int) error {
	return c.Status(fiber.StatusOK).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
		Meta:    &Meta{Page: page, Limit: limit, Total: total},
	})
}
