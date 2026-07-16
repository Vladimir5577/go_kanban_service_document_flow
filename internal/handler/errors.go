package handler

import (
	"errors"

	gpvalidator "github.com/go-playground/validator/v10"

	"go_kanban_service/internal/apperr"
)

type validationCodeKey struct {
	Field string
	Tag   string
}

func invalidJSONError() error {
	return apperr.New(apperr.CodeInvalidJSON, "invalid json")
}

func validationError(err error, codes map[validationCodeKey]apperr.ErrorCode) error {
	var validationErrors gpvalidator.ValidationErrors
	if errors.As(err, &validationErrors) {
		for _, fieldErr := range validationErrors {
			if code, ok := codes[validationCodeKey{Field: fieldErr.Field(), Tag: fieldErr.Tag()}]; ok {
				return apperr.New(code, string(code))
			}
		}
	}
	return apperr.New(apperr.CodeValidation, "validation failed")
}
