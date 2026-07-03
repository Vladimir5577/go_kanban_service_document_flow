package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/repository/dbgen"
)

// ExecTx выполняет функцию fn в рамках транзакции.
// Если fn возвращает ошибку, происходит Rollback, иначе Commit.
func ExecTx(ctx context.Context, db *pgxpool.Pool, fn func(*dbgen.Queries) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	
	// Ensure rollback on panic or return error
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	q := dbgen.New(tx)
	if err := fn(q); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
