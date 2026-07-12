package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/repository/dbgen"
)

// ExecTx выполняет функцию fn в рамках транзакции.
// Гарантирует Rollback при ошибке, панике или если Commit не был вызван.
// Используйте этот хелпер для всех операций с каскадами, чтобы избежать
// частично применённых изменений (целостность данных).
func ExecTx(ctx context.Context, db *pgxpool.Pool, fn func(*dbgen.Queries) error) (err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	committed := false
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) // re-panic after rollback
		}
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	q := dbgen.New(tx)
	if err = fn(q); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	committed = true
	return nil
}
