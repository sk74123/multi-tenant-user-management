package common

import (
	"encoding/json"
	"net/http"
)

// Meta holds pagination metadata included in list responses.
type Meta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

// Response is the standard JSON envelope for all API responses.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

// ErrorBody is the structured error payload.
type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// OK writes a successful 200 response.
func OK(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, Response{Success: true, Data: data})
}

// OKWithMeta writes a successful 200 response with pagination metadata.
func OKWithMeta(w http.ResponseWriter, data interface{}, meta Meta) {
	JSON(w, http.StatusOK, Response{Success: true, Data: data, Meta: &meta})
}

// Created writes a 201 response.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, Response{Success: true, Data: data})
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error writes a structured error response derived from an AppError.
func Error(w http.ResponseWriter, err *AppError) {
	JSON(w, err.HTTPStatus, Response{
		Success: false,
		Error:   &ErrorBody{Code: err.Code, Message: err.Message},
	})
}

// ValidationError writes a 422 with field-level detail.
func ValidationError(w http.ResponseWriter, details interface{}) {
	JSON(w, http.StatusUnprocessableEntity, Response{
		Success: false,
		Error: &ErrorBody{
			Code:    "VALIDATION_ERROR",
			Message: "validation failed",
			Details: details,
		},
	})
}
