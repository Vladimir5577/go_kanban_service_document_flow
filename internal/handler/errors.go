package handler

import (
	"errors"
	"log/slog"
	"net/http"

	gpvalidator "github.com/go-playground/validator/v10"
	"github.com/minio/minio-go/v7"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/helper"
)

type validationCodeKey struct {
	Field string
	Tag   string
}

func invalidJSONError() error {
	return apperr.New(apperr.CodeInvalidJSON, "invalid json")
}

func writeAttachmentStorageError(w http.ResponseWriter, err error) {
	var storageErr minio.ErrorResponse
	if errors.As(err, &storageErr) && storageErr.Code == "NoSuchKey" {
		helper.WriteError(w, apperr.New(apperr.CodeFileNotFoundOnDisk, "file not found on disk"))
		return
	}
	slog.Error("failed to read attachment from storage", "error", err)
	helper.WriteJSON(w, http.StatusBadGateway, map[string]string{
		"error": "internal_error",
		"code":  "internal_error",
	})
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
