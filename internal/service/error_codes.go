package service

import (
	"errors"

	"go_kanban_service/internal/apperr"
)

func withNotFoundCode(err error, code apperr.ErrorCode) error {
	if errors.Is(err, apperr.ErrNotFound) {
		return apperr.New(code, string(code))
	}
	return err
}

func accessDenied() error {
	return apperr.New(apperr.CodeAccessDenied, "access denied")
}
