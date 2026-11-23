// Package httpx contains HTTP plumbing shared by every module: the error
// envelope, request binding/validation, and middleware.
//
// Error envelope (all non-2xx responses):
//
//	{"error": {"code": "validation_failed", "message": "…", "fields": {"email": "email"}}}
//
// `code` is stable and machine-readable (clients switch on it); `message` is for humans.
package httpx

import (
	"errors"
	"log/slog"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

type APIError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func Err(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

var (
	ErrUnauthorized = Err(401, "unauthorized", "Authentication required")
	ErrForbidden    = Err(403, "forbidden", "You do not have access to this resource")
	ErrNotFound     = Err(404, "not_found", "Resource not found")
	ErrConflict     = Err(409, "conflict", "The resource was modified by someone else")
	ErrRateLimited  = Err(429, "rate_limited", "Too many requests, slow down")
)

// ErrorHandler is installed as fiber.Config.ErrorHandler. Internal errors are
// logged with the request id and never leaked to clients.
func ErrorHandler(log *slog.Logger) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		var ae *APIError
		var fe *fiber.Error
		switch {
		case errors.As(err, &ae):
		case errors.As(err, &fe):
			ae = Err(fe.Code, codeFor(fe.Code), fe.Message)
		default:
			log.Error("unhandled error", "err", err, "path", c.Path(), "request_id", c.RequestID())
			ae = Err(500, "internal", "Something went wrong")
		}
		return c.Status(ae.Status).JSON(fiber.Map{"error": ae})
	}
}

func codeFor(status int) string {
	switch status {
	case 400:
		return "bad_request"
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 405:
		return "method_not_allowed"
	case 413:
		return "payload_too_large"
	case 429:
		return "rate_limited"
	default:
		if status >= 500 {
			return "internal"
		}
		return "error"
	}
}

var validate = validator.New(validator.WithRequiredStructEnabled())

// Bind parses the JSON body into dst and runs `validate:"…"` struct tags.
func Bind(c fiber.Ctx, dst any) error {
	if err := c.Bind().Body(dst); err != nil {
		return Err(400, "bad_request", "Malformed request body")
	}
	return Validate(dst)
}

func Validate(v any) error {
	if err := validate.Struct(v); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			fields := make(map[string]string, len(ve))
			for _, f := range ve {
				fields[f.Field()] = f.Tag()
			}
			return &APIError{Status: 422, Code: "validation_failed", Message: "Some fields are invalid", Fields: fields}
		}
		return Err(400, "bad_request", err.Error())
	}
	return nil
}
