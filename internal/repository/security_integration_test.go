package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

func TestSecurityR07UndoRejectsFormerAssignee(t *testing.T) {
	db := securityDB(t)
	securityFixture(t, db)
	ctx := context.Background()
	if err := NewProjectMemberRepository(db).RemoveMember(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	err := NewHistoryRepository(db).ApplyUndo(ctx, model.HistoryUndoEntry{
		ProjectID: 1, EntityID: 1, Action: "card.assignee_removed", Before: "[2]",
	})
	if err == nil {
		t.Fatal("undo accepted former member")
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM kanban_card_assignee WHERE card_id=1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("assignees=%d", count)
	}
}

func TestSecurityR13RestoreOnlyChildrenDeletedWithParent(t *testing.T) {
	db := securityDB(t)
	securityFixture(t, db)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO kanban_card(id,title,parent_id) VALUES(3,'live child',1)`); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(db)
	if _, err := q.DeleteCard(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DeleteCard(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// Reproduce the timestamp collision regardless of execution speed.
	if _, err := db.Exec(ctx, `UPDATE kanban_card SET deleted_at='2026-10-11 12:00:00+00'`); err != nil {
		t.Fatal(err)
	}
	if err := q.RestoreCard(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetCard(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetCard(ctx, 3); err != nil {
		t.Fatal("cascade child was not restored", err)
	}
	if _, err := q.GetCard(ctx, 2); err == nil {
		t.Fatal("separately deleted child restored")
	}
}

// Only an explicitly supplied local test database is used. Each test owns a schema.
func securityDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("KANBAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("не запускался: KANBAN_TEST_DATABASE_URL не задан (нужна локальная PostgreSQL)")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "security_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(string(body), "-- +goose Down")[0]
		if _, err := db.Exec(ctx, up); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	return db
}

func securityFixture(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
	INSERT INTO users (id,login,lastname,firstname) VALUES (1,'owner','Owner','One'),(2,'editor','Editor','Two');
	INSERT INTO kanban_project (id,name,owner_id) VALUES (1,'project',1);
	INSERT INTO kanban_project_user (kanban_project_id,user_id,role) VALUES (1,1,'KANBAN_ADMIN'),(1,2,'KANBAN_EDITOR');
	INSERT INTO kanban_board (id,title,kanban_project_id,created_by_id) VALUES (1,'board',1,1);
	INSERT INTO kanban_column (id,title,board_id,header_color) VALUES (1,'column',1,'gray');
	INSERT INTO kanban_card (id,title,column_id) VALUES (1,'root',1);
	INSERT INTO kanban_card (id,title,parent_id) VALUES (2,'child',1);`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSecurityR01UserDeletionRevokesStatus(t *testing.T) {
	db := securityDB(t)
	securityFixture(t, db)
	r := NewUserRepository(db, nil, helper.NewClock())
	ctx := context.Background()
	active, err := r.IsUserActive(ctx, 1)
	if err != nil || !active {
		t.Fatalf("active=%v err=%v", active, err)
	}
	if _, err := db.Exec(ctx, `UPDATE users SET deleted_at=NOW() WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 999} {
		active, err := r.IsUserActive(ctx, id)
		if err != nil || active {
			t.Fatalf("id=%d active=%v err=%v", id, active, err)
		}
	}
}

func TestSecurityR03DeletedHierarchyIsInaccessible(t *testing.T) {
	for _, table := range []string{"kanban_card", "kanban_column", "kanban_board"} {
		t.Run(table, func(t *testing.T) {
			db := securityDB(t)
			securityFixture(t, db)
			ctx := context.Background()
			q := dbgen.New(db)
			if _, err := q.GetCardContext(ctx, dbgen.GetCardContextParams{ID: 2, UserID: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ctx, "UPDATE "+table+" SET deleted_at=NOW() WHERE id=1"); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{1, 2} {
				if _, err := q.GetCardContext(ctx, dbgen.GetCardContextParams{ID: id, UserID: 1}); err == nil {
					t.Fatalf("deleted hierarchy visible for card %d", id)
				}
				if _, err := q.GetProjectIDByCard(ctx, id); err == nil {
					t.Fatalf("deleted hierarchy resolved for card %d", id)
				}
			}
			if table != "kanban_card" {
				if _, err := q.GetColumnContext(ctx, dbgen.GetColumnContextParams{ID: 1, UserID: 1}); err == nil {
					t.Fatal("deleted column hierarchy visible")
				}
			}
		})
	}
}
