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
		message = code
		status = statusForErrorCode(appErr.Code)
	} else {
		// Fallback for wrapped basic errors (if they somehow exist)
		switch {
		case errors.Is(err, apperr.ErrNotFound):
			status = http.StatusNotFound
			code = string(apperr.CodeNotFound)
			message = code
		case errors.Is(err, apperr.ErrForbidden):
			status = http.StatusForbidden
			code = string(apperr.CodeForbidden)
			message = code
		case errors.Is(err, apperr.ErrUnauthorized):
			status = http.StatusUnauthorized
			code = string(apperr.CodeUnauthorized)
			message = code
		case errors.Is(err, apperr.ErrConflict):
			status = http.StatusConflict
			code = string(apperr.CodeConflict)
			message = code
		case errors.Is(err, apperr.ErrValidation):
			status = http.StatusBadRequest
			code = string(apperr.CodeValidation)
			message = code
		}
	}

	WriteJSON(w, status, map[string]string{
		"error": message,
		"code":  code,
	})
}

func statusForErrorCode(code apperr.ErrorCode) int {
	switch code {
	case apperr.CodeNotFound,
		apperr.CodeAttachmentNotFound,
		apperr.CodeAttachmentNotPreviewable,
		apperr.CodeBoardNotFound,
		apperr.CodeCardNotFound,
		apperr.CodeColumnNotFound,
		apperr.CodeCommentNotFound,
		apperr.CodeFileNotFoundOnDisk,
		apperr.CodeFolderNotFound,
		apperr.CodeLabelNotFound,
		apperr.CodeMemberNotFound,
		apperr.CodeProjectNotFound,
		apperr.CodeSubtaskNotFound,
		apperr.CodeUserNotFound:
		return http.StatusNotFound
	case apperr.CodeForbidden,
		apperr.CodeAccessDenied,
		apperr.CodeCommentAuthorOnly,
		apperr.CodeInsufficientPermissions,
		apperr.CodeProjectAccessDenied:
		return http.StatusForbidden
	case apperr.CodeUnauthorized:
		return http.StatusUnauthorized
	case apperr.CodeConflict,
		apperr.CodeBoardCardLimitReached,
		apperr.CodeBoardHasCards,
		apperr.CodeColumnHasCards,
		apperr.CodeCommentLimitReached:
		return http.StatusConflict
	case apperr.CodeValidation,
		apperr.CodeAttachmentLimitReached,
		apperr.CodeBoardHasNoProject,
		apperr.CodeBoardTitleRequired,
		apperr.CodeBoardTitleTooLong,
		apperr.CodeCannotRemoveOwner,
		apperr.CodeCannotRemoveSelf,
		apperr.CodeColumnIDAndPositionRequired,
		apperr.CodeColumnIDAndTitleRequired,
		apperr.CodeColumnTitleRequired,
		apperr.CodeCommentBodyRequired,
		apperr.CodeCommentBodyTooLong,
		apperr.CodeDescriptionInvalidType,
		apperr.CodeFileNotProvided,
		apperr.CodeFolderNameRequired,
		apperr.CodeFolderNameTooLong,
		apperr.CodeInvalidJSON,
		apperr.CodeInvalidRole,
		apperr.CodeInvalidRoleForUser,
		apperr.CodeLabelNameRequired,
		apperr.CodeMembersArrayExpected,
		apperr.CodeMembersListEmpty,
		apperr.CodeOwnerRoleImmutable,
		apperr.CodeProjectNameRequired,
		apperr.CodeProjectNameTooLong,
		apperr.CodeSubtaskTitleRequired,
		apperr.CodeUpdateFieldsRequired,
		apperr.CodeUserNotProjectMember:
		return http.StatusBadRequest
	case apperr.CodeProjectCreateFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
