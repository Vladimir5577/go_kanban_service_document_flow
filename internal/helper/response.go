// Package helper содержит утилиты HTTP-слоя: единообразную запись JSON-ответов
// и маппинг доменных ошибок (apperr) в HTTP-статусы.
package helper

import (
	"encoding/json"
	"errors"
	"net/http"

	"go_kanban_service/internal/apperr"
)

// WriteJSON пишет data как JSON с указанным статусом.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Заголовки уже отправлены — только логировать смысла нет, ответ уже частично ушёл.
		return
	}
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	code := "internal_error"

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		code = string(appErr.Code)
		switch appErr.Code {
		case apperr.CodeNotFound:
			status = http.StatusNotFound
			message = "not found"
		case apperr.CodeForbidden:
			status = http.StatusForbidden
			message = "forbidden"
		case apperr.CodeUnauthorized:
			status = http.StatusUnauthorized
			message = "unauthorized"
		case apperr.CodeConflict:
			status = http.StatusConflict
			message = "conflict"
		case apperr.CodeValidation:
			status = http.StatusBadRequest
			message = err.Error()
		}
	} else {
		// Fallback for wrapped basic errors (if they somehow exist)
		switch {
		case errors.Is(err, apperr.ErrNotFound):
			status = http.StatusNotFound
			message = "not found"
			code = string(apperr.CodeNotFound)
		case errors.Is(err, apperr.ErrForbidden):
			status = http.StatusForbidden
			message = "forbidden"
			code = string(apperr.CodeForbidden)
		case errors.Is(err, apperr.ErrUnauthorized):
			status = http.StatusUnauthorized
			message = "unauthorized"
			code = string(apperr.CodeUnauthorized)
		case errors.Is(err, apperr.ErrConflict):
			status = http.StatusConflict
			message = "conflict"
			code = string(apperr.CodeConflict)
		case errors.Is(err, apperr.ErrValidation):
			status = http.StatusBadRequest
			message = err.Error()
			code = string(apperr.CodeValidation)
		}
	}

	WriteJSON(w, status, map[string]string{
		"error": message,
		"code":  code,
	})
}
