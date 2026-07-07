package repository

import (
	"errors"

	"github.com/jackc/pgx/v5"

	"go_kanban_service/internal/apperr"
)

func NormalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrNotFound
	}
	return err
}
