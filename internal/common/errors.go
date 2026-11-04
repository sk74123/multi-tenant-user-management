package common

import (
	"errors"
	"net/http"
)

// AppError is a structured application error with an HTTP status and machine-
// readable code.
type AppError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *AppError) Error() string { return e.Message }

// Sentinel errors — use errors.Is() to check.
var (
	ErrNotFound     = &AppError{HTTPStatus: http.StatusNotFound, Code: "NOT_FOUND", Message: "resource not found"}
	ErrUnauthorized = &AppError{HTTPStatus: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "authentication required"}
	ErrForbidden    = &AppError{HTTPStatus: http.StatusForbidden, Code: "FORBIDDEN", Message: "insufficient permissions"}
	ErrConflict     = &AppError{HTTPStatus: http.StatusConflict, Code: "CONFLICT", Message: "resource already exists"}
	ErrBadRequest   = &AppError{HTTPStatus: http.StatusBadRequest, Code: "BAD_REQUEST", Message: "invalid request"}
	ErrInternal     = &AppError{HTTPStatus: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "an internal error occurred"}
)

// New creates a new AppError with custom message.
func New(httpStatus int, code, message string) *AppError {
	return &AppError{HTTPStatus: httpStatus, Code: code, Message: message}
}

// AsAppError extracts an *AppError from err, or returns ErrInternal.
func AsAppError(err error) *AppError {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return ErrInternal
}
