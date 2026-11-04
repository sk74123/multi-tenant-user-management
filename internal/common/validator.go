package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// Decode decodes the JSON body of r into dst and validates the struct tags.
// On error it writes the appropriate response and returns false.
func Decode(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		Error(w, &AppError{
			HTTPStatus: http.StatusBadRequest,
			Code:       "INVALID_JSON",
			Message:    "request body is not valid JSON",
		})
		return false
	}

	if err := validate.Struct(dst); err != nil {
		var errs validator.ValidationErrors
		if ok := isValidationErrors(err, &errs); ok {
			details := make([]map[string]string, 0, len(errs))
			for _, fe := range errs {
				details = append(details, map[string]string{
					"field":   toSnakeCase(fe.Field()),
					"message": validationMessage(fe),
				})
			}
			ValidationError(w, details)
			return false
		}
		Error(w, ErrBadRequest)
		return false
	}

	return true
}

func isValidationErrors(err error, out *validator.ValidationErrors) bool {
	switch v := err.(type) {
	case validator.ValidationErrors:
		*out = v
		return true
	}
	return false
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", toSnakeCase(fe.Field()))
	case "email":
		return fmt.Sprintf("%s must be a valid email", toSnakeCase(fe.Field()))
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", toSnakeCase(fe.Field()), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", toSnakeCase(fe.Field()), fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", toSnakeCase(fe.Field()), fe.Param())
	default:
		return fmt.Sprintf("%s failed validation: %s", toSnakeCase(fe.Field()), fe.Tag())
	}
}

// toSnakeCase converts "FirstName" → "first_name".
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
