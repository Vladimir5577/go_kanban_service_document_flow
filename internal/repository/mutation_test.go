package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type mutationTxStub struct {
	pgx.Tx
	commits, rollbacks, begins int
	commitErr                  error
	child                      *mutationTxStub
}

func (s *mutationTxStub) Commit(context.Context) error {
	s.commits++
	return s.commitErr
}

func (s *mutationTxStub) Rollback(context.Context) error {
	s.rollbacks++
	return nil
}

func (s *mutationTxStub) Begin(context.Context) (pgx.Tx, error) {
	s.begins++
	s.child = &mutationTxStub{}
	return s.child, nil
}

func TestMutationCommitAndFailureEffects(t *testing.T) {
	failure := errors.New("history write failed")
	for _, scenario := range []string{"commit", "return error", "history failure", "commit error"} {
		t.Run(scenario, func(t *testing.T) {
			tx := &mutationTxStub{}
			if scenario == "commit error" {
				tx.commitErr = failure
			}
			called := false
			type requestKey struct{}
			ctx := context.WithValue(context.Background(), requestKey{}, "request")
			err := withMutationTx(ctx, tx, func(ctx context.Context) error {
				if !InMutation(ctx) || Connection(ctx, nil) != tx {
					t.Fatal("repositories must share the active transaction")
				}
				if !AfterCommit(ctx, func(clean context.Context) {
					called = true
					if tx.commits != 1 || tx.rollbacks != 0 {
						t.Fatal("effect ran before commit")
					}
					if InMutation(clean) || AfterCommit(clean, func(context.Context) {}) {
						t.Fatal("effect retained transaction state")
					}
					if clean.Value(requestKey{}) != "request" {
						t.Fatal("effect lost request values")
					}
				}) {
					t.Fatal("effect was not queued")
				}
				if called {
					t.Fatal("effect ran before mutation completed")
				}
				switch scenario {
				case "return error":
					return failure
				case "history failure":
					FailMutation(ctx, failure)
					FailMutation(ctx, nil)
				}
				return nil
			})
			if scenario == "commit" {
				if err != nil || !called || tx.commits != 1 || tx.rollbacks != 0 {
					t.Fatalf("err=%v effect=%v commits=%d rollbacks=%d", err, called, tx.commits, tx.rollbacks)
				}
			} else if !errors.Is(err, failure) || called || tx.rollbacks != 1 {
				t.Fatalf("err=%v effect=%v commits=%d rollbacks=%d", err, called, tx.commits, tx.rollbacks)
			}
			if scenario != "commit" && scenario != "commit error" && tx.commits != 0 {
				t.Fatal("failed mutation attempted commit")
			}
		})
	}
}

func TestMutationPanicRollsBack(t *testing.T) {
	tx := &mutationTxStub{}
	called := false
	defer func() {
		if recover() != "failed mutation" || tx.rollbacks != 1 || tx.commits != 0 || called {
			t.Fatalf("panic rollback failed: commits=%d rollbacks=%d effect=%v", tx.commits, tx.rollbacks, called)
		}
	}()
	_ = withMutationTx(context.Background(), tx, func(ctx context.Context) error {
		AfterCommit(ctx, func(context.Context) { called = true })
		panic("failed mutation")
	})
}

func TestMutationNestedWorkReusesTransactionAndSavepoint(t *testing.T) {
	tx := &mutationTxStub{}
	err := withMutationTx(context.Background(), tx, func(ctx context.Context) error {
		return WithMutation(ctx, nil, func(ctx context.Context) error {
			if Connection(ctx, nil) != tx {
				t.Fatal("nested mutation replaced the outer transaction")
			}
			savepoint, err := Begin(ctx, nil)
			if err != nil {
				return err
			}
			if tx.begins != 1 || savepoint != tx.child || tx.commits != 0 {
				t.Fatal("nested Begin did not open a savepoint")
			}
			return savepoint.Commit(ctx)
		})
	})
	if err != nil || tx.commits != 1 || tx.child.commits != 1 || tx.rollbacks != 0 {
		t.Fatalf("err=%v commits=%d rollbacks=%d", err, tx.commits, tx.rollbacks)
	}
}

func TestMutationWithoutDatabaseCallsMock(t *testing.T) {
	called := false
	err := WithMutation(context.Background(), nil, func(ctx context.Context) error {
		called = true
		if InMutation(ctx) {
			t.Fatal("nil database must not create transaction state")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestSecurityR08MutationRollbackAndSavepoint(t *testing.T) {
	db := securityDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, "CREATE TABLE mutation_probe (id int PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("history unavailable")
	effect := false
	err := WithMutation(ctx, db, func(ctx context.Context) error {
		if _, err := Connection(ctx, db).Exec(ctx, "INSERT INTO mutation_probe VALUES (1)"); err != nil {
			return err
		}
		child, err := Begin(ctx, db)
		if err != nil {
			return err
		}
		if _, err = child.Exec(ctx, "INSERT INTO mutation_probe VALUES (2)"); err != nil {
			return err
		}
		if err := child.Commit(ctx); err != nil {
			return err
		}
		AfterCommit(ctx, func(context.Context) { effect = true })
		FailMutation(ctx, failure)
		return nil
	})
	if !errors.Is(err, failure) || effect {
		t.Fatalf("err=%v effect=%v", err, effect)
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM mutation_probe").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left rows: count=%d err=%v", count, err)
	}
	err = WithMutation(ctx, db, func(ctx context.Context) error {
		if _, err := Connection(ctx, db).Exec(ctx, "INSERT INTO mutation_probe VALUES (3)"); err != nil {
			return err
		}
		AfterCommit(ctx, func(clean context.Context) {
			if InMutation(clean) {
				t.Fatal("committed effect retained transaction")
			}
			if err := db.QueryRow(clean, "SELECT count(*) FROM mutation_probe").Scan(&count); err != nil || count != 1 {
				t.Fatalf("effect cannot see committed row: count=%d err=%v", count, err)
			}
			effect = true
		})
		return nil
	})
	if err != nil || !effect {
		t.Fatalf("err=%v effect=%v", err, effect)
	}
}
