package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/repository/dbgen"
)

type mutationKey struct{}

type mutationState struct {
	tx          pgx.Tx
	err         error
	afterCommit []func()
}

func mutationFromContext(ctx context.Context) *mutationState {
	state, _ := ctx.Value(mutationKey{}).(*mutationState)
	return state
}

func InMutation(ctx context.Context) bool {
	return mutationFromContext(ctx) != nil
}

// WithMutation keeps a business change and its history in the same transaction.
func WithMutation(ctx context.Context, db *pgxpool.Pool, fn func(context.Context) error) error {
	if InMutation(ctx) {
		err := fn(ctx)
		FailMutation(ctx, err)
		return err
	}
	if db == nil {
		return fn(ctx)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin mutation: %w", err)
	}
	return withMutationTx(ctx, tx, fn)
}

func withMutationTx(ctx context.Context, tx pgx.Tx, fn func(context.Context) error) error {
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	state := &mutationState{tx: tx}
	ctx = context.WithValue(ctx, mutationKey{}, state)
	if err := fn(ctx); err != nil {
		return err
	}
	if state.err != nil {
		return state.err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit mutation: %w", err)
	}
	committed = true
	for _, callback := range state.afterCommit {
		callback()
	}
	return nil
}

func Connection(ctx context.Context, db *pgxpool.Pool) dbgen.DBTX {
	if state := mutationFromContext(ctx); state != nil {
		return state.tx
	}
	return db
}

func Queries(ctx context.Context, db *pgxpool.Pool) *dbgen.Queries {
	return dbgen.New(Connection(ctx, db))
}

// Begin uses a savepoint when the caller already owns the outer transaction.
func Begin(ctx context.Context, db *pgxpool.Pool) (pgx.Tx, error) {
	if state := mutationFromContext(ctx); state != nil {
		return state.tx.Begin(ctx)
	}
	return db.Begin(ctx)
}

// FailMutation preserves errors from history calls whose callers cannot return them.
func FailMutation(ctx context.Context, err error) {
	if state := mutationFromContext(ctx); state != nil && state.err == nil && err != nil {
		state.err = err
	}
}

// AfterCommit defers effects and removes transaction state from their context.
func AfterCommit(ctx context.Context, fn func(context.Context)) bool {
	state := mutationFromContext(ctx)
	if state == nil {
		return false
	}
	if fn != nil {
		state.afterCommit = append(state.afterCommit, func() {
			fn(context.WithValue(ctx, mutationKey{}, (*mutationState)(nil)))
		})
	}
	return true
}
