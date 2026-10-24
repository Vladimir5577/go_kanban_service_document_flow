package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/repository"
)

type securityServices struct {
	db          *pgxpool.Pool
	cards       *CardService
	comments    *CommentService
	attachments *AttachmentService
	projects    *ProjectService
	columns     *ColumnService
	boards      *BoardService
	history     *HistoryService
}

func securityServicesDB(t *testing.T) securityServices {
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
	schema := "service_security_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	if cfg.MaxConns < 3 {
		cfg.MaxConns = 3 // Two mutations and an independent lock observer.
	}
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
		if _, err = db.Exec(ctx, strings.Split(string(body), "-- +goose Down")[0]); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	securityExec(t, db, `INSERT INTO users(id,login,lastname,firstname) VALUES(1,'one','One','One'),(2,'two','Two','Two');
	INSERT INTO kanban_project(id,name,owner_id) VALUES(1,'project',1);
	INSERT INTO kanban_project_user(kanban_project_id,user_id,role) VALUES(1,1,'KANBAN_ADMIN'),(1,2,'KANBAN_EDITOR');
	INSERT INTO kanban_board(id,title,kanban_project_id,created_by_id) VALUES(1,'board',1,1);
	INSERT INTO kanban_column(id,title,board_id,position) VALUES(1,'column',1,1);
	INSERT INTO kanban_card(id,title,column_id,created_by_id) VALUES(1,'old',1,1);
	SELECT setval('kanban_project_id_seq',1); SELECT setval('kanban_board_id_seq',1);
	SELECT setval('kanban_column_id_seq',1); SELECT setval('kanban_card_id_seq',1);`)
	p := repository.NewProjectRepository(db)
	m := repository.NewProjectMemberRepository(db)
	b := repository.NewBoardRepository(db)
	col := repository.NewColumnRepository(db)
	c := repository.NewCardRepository(db)
	com := repository.NewCommentRepository(db)
	reads := repository.NewCommentReadRepository(db)
	att := repository.NewAttachmentRepository(db)
	labels := repository.NewLabelRepository(db)
	u := repository.NewUserRepository(db, nil, helper.NewClock())
	perm := NewPermissionService(db, p, m)
	appCfg := &config.Config{Clock: helper.NewClock()}
	h := NewHistoryService(repository.NewHistoryRepository(db), m, perm, nil, c, col)
	s := securityServices{db: db, history: h,
		cards:       NewCardService(c, perm, nil, com, reads, att, labels, u, col, b, m, nil, nil, appCfg),
		comments:    NewCommentService(com, reads, perm, u, nil, nil),
		attachments: NewAttachmentService(att, perm, nil, u),
		projects:    NewProjectService(p, b, m, repository.NewProjectFolderRepository(db), u, perm, appCfg),
		columns:     NewColumnService(col, perm, b),
		boards:      NewBoardService(b, col, c, labels, u, com, att, perm, appCfg),
	}
	s.cards.History = h
	s.comments.History = h
	s.attachments.History = h
	s.projects.History = h
	s.columns.History = h
	s.boards.History = h
	return s
}

func securityExec(t *testing.T, db *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
}
func securityUser(id int64) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, middleware.UserClaims{ID: id})
}
func securityParallel(a, b func() error) []error {
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, fn := range []func() error{a, b} {
		wg.Add(1)
		go func(i int, fn func() error) { defer wg.Done(); <-start; errs[i] = fn() }(i, fn)
	}
	close(start)
	wg.Wait()
	return errs
}
func requireBoth(t *testing.T, errs []error) {
	t.Helper()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSecurityR08UndoDoesNotLoseConcurrentWrite(t *testing.T) {
	s := securityServicesDB(t)
	a, b := "A", "B"
	if _, err := s.cards.UpdateCard(securityUser(1), 1, dto.UpdateCardRequest{Title: &a}); err != nil {
		t.Fatal(err)
	}
	barrier := newSecurityBarrier(s.db)
	s.history.repo = &securityHistoryBarrier{HistoryRepositoryInterface: s.history.repo, barrier: barrier}
	errs := securityInterleave(t, s.db, barrier, func() error { _, _, _, err := s.history.Undo(securityUser(1), 1); return err }, func() error {
		_, err := s.cards.UpdateCard(securityUser(2), 1, dto.UpdateCardRequest{Title: &b})
		return err
	})
	if errs[1] != nil {
		t.Fatal(errs[1])
	}
	if errs[0] != nil && !errors.Is(errs[0], apperr.New(apperr.CodeUndoImpossible, "")) {
		t.Fatalf("unexpected undo error: %v", errs[0])
	}
	card, err := s.cards.repo.GetCardRow(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "B" {
		t.Fatalf("foreign change lost: %s (undo=%v)", card.Title, errs[0])
	}
}

func TestSecurityR08HistoryFailureRollsBackMutation(t *testing.T) {
	s := securityServicesDB(t)
	securityExec(t, s.db, `CREATE FUNCTION reject_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test history failure'; END $$;
	CREATE TRIGGER reject_history BEFORE INSERT ON kanban_project_history FOR EACH ROW EXECUTE FUNCTION reject_history();`)
	title := "new"
	if _, err := s.cards.UpdateCard(securityUser(1), 1, dto.UpdateCardRequest{Title: &title}); err == nil {
		t.Fatal("history failure accepted")
	}
	card, err := s.cards.repo.GetCardRow(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != "old" {
		t.Fatalf("partial mutation: %s", card.Title)
	}
}

func TestSecurityR09ConcurrentPatchesPreserveBothFields(t *testing.T) {
	s := securityServicesDB(t)
	ctx := securityUser(1)
	title, desc := "new", "description"
	pos := float64(42)
	cardBarrier := newSecurityBarrier(s.db)
	s.cards.repo = &securityCardBarrier{CardRepositoryInterface: s.cards.repo, row: cardBarrier}
	requireBoth(t, securityInterleave(t, s.db, cardBarrier, func() error { _, err := s.cards.UpdateCard(ctx, 1, dto.UpdateCardRequest{Title: &title}); return err }, func() error {
		_, err := s.cards.UpdateCard(ctx, 1, dto.UpdateCardRequest{HasDescription: true, Description: &desc})
		return err
	}))
	card, err := s.cards.repo.GetCardRow(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if card.Title != title || card.Description == nil || *card.Description != desc {
		t.Fatalf("card=%+v", card)
	}
	columnBarrier := newSecurityBarrier(s.db)
	s.columns.repo = &securityColumnBarrier{ColumnRepositoryInterface: s.columns.repo, barrier: columnBarrier}
	requireBoth(t, securityInterleave(t, s.db, columnBarrier, func() error {
		_, _, err := s.columns.UpdateColumn(ctx, 1, 1, 1, dto.UpdateColumnRequest{Title: &title})
		return err
	}, func() error {
		_, _, err := s.columns.UpdateColumn(ctx, 1, 1, 1, dto.UpdateColumnRequest{Position: &pos})
		return err
	}))
	column, err := s.columns.repo.GetColumn(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if column.Title != title || column.Position != pos {
		t.Fatalf("column=%+v", column)
	}
	requireBoth(t, securityParallel(func() error {
		_, err := s.projects.UpdateProject(ctx, 1, dto.UpdateProjectRequest{Name: &title})
		return err
	}, func() error {
		_, err := s.projects.UpdateProject(ctx, 1, dto.UpdateProjectRequest{Description: &desc})
		return err
	}))
	p, err := s.projects.repo.GetProject(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != title || p.Description == nil || *p.Description != desc {
		t.Fatalf("project=%+v", p)
	}
	requireBoth(t, securityParallel(func() error {
		_, err := s.boards.UpdateBoard(ctx, 1, 1, dto.UpdateBoardRequest{Title: &title})
		return err
	}, func() error {
		_, err := s.boards.UpdateBoard(ctx, 1, 1, dto.UpdateBoardRequest{Position: &pos})
		return err
	}))
	board, err := s.boards.repo.GetBoard(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if board.Title != title || board.Position != pos {
		t.Fatalf("board=%+v", board)
	}
}

func TestSecurityR10ConcurrentAssigneesRemainSingular(t *testing.T) {
	s := securityServicesDB(t)
	ctx := securityUser(1)
	requireBoth(t, securityParallel(func() error { _, err := s.cards.UpdateAssignees(ctx, 1, []int64{1}); return err }, func() error { _, err := s.cards.UpdateAssignees(ctx, 1, []int64{2}); return err }))
	ids, err := s.cards.repo.GetAssigneesByCardIDs(ctx, []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids[1]) != 1 {
		t.Fatalf("assignees=%v", ids[1])
	}
}

func TestSecurityProjectMovesSerializeRebalanceAcrossProjects(t *testing.T) {
	s := securityServicesDB(t)
	securityExec(t, s.db, `INSERT INTO kanban_project(id,name,owner_id) VALUES(2,'second',1);
	INSERT INTO kanban_project_user(kanban_project_id,user_id,role,position) VALUES(2,1,'KANBAN_ADMIN',0)`)
	ctx, cancel := context.WithTimeout(securityUser(1), 10*time.Second)
	defer cancel()
	barrier := newSecurityBarrier(s.db)
	s.projects.memberRepo = &securityProjectMemberBarrier{ProjectMemberRepositoryInterface: s.projects.memberRepo, barrier: barrier}
	position := float64(0)
	requireBoth(t, securityInterleave(t, s.db, barrier, func() error {
		_, err := s.projects.MoveProject(ctx, 1, dto.MoveProjectRequest{Position: &position})
		return err
	}, func() error {
		_, err := s.projects.MoveProject(ctx, 2, dto.MoveProjectRequest{Position: &position})
		return err
	}))
	var total, distinctPositions int
	if err := s.db.QueryRow(ctx, "SELECT count(*), count(DISTINCT position) FROM kanban_project_user WHERE user_id=1").Scan(&total, &distinctPositions); err != nil {
		t.Fatal(err)
	}
	if total != 2 || distinctPositions != 2 {
		t.Fatalf("project placements: total=%d distinct positions=%d", total, distinctPositions)
	}
}

func TestSecurityR11ConcurrentLimits(t *testing.T) {
	for _, kind := range []string{"attachments", "comments", "children", "cards"} {
		t.Run(kind, func(t *testing.T) {
			s := securityServicesDB(t)
			ctx := securityUser(1)
			one := int64(1)
			barrier := newSecurityBarrier(s.db)
			var create func() error
			var wantCode apperr.ErrorCode
			var wantCount int
			var countSQL string
			switch kind {
			case "attachments":
				securityExec(t, s.db, `INSERT INTO kanban_attachment(filename,storage_key,content_type,size_bytes,card_id,context) SELECT 'f','k','text/plain',1,1,'info' FROM generate_series(1,15)`)
				s.attachments.repo = &securityAttachmentBarrier{AttachmentRepositoryInterface: s.attachments.repo, barrier: barrier}
				wantCode, wantCount = apperr.CodeAttachmentLimitReached, 16
				countSQL = "SELECT count(*) FROM kanban_attachment WHERE card_id=1 AND context='info' AND deleted_at IS NULL"
				create = func() error {
					_, err := s.attachments.CreateAttachment(ctx, 1, dto.CreateAttachmentRequest{Filename: "f", StorageKey: "key", ContentType: "text/plain", SizeBytes: 1, Context: "info"})
					return err
				}
			case "comments":
				securityExec(t, s.db, `INSERT INTO kanban_card_comment(body,card_id,author_id) SELECT 'c',1,1 FROM generate_series(1,299)`)
				s.comments.repo = &securityCommentBarrier{CommentRepositoryInterface: s.comments.repo, barrier: barrier}
				wantCode, wantCount = apperr.CodeCommentLimitReached, 300
				countSQL = "SELECT count(*) FROM kanban_card_comment WHERE card_id=1 AND deleted_at IS NULL"
				create = func() error {
					_, err := s.comments.CreateComment(ctx, 1, dto.CreateCommentRequest{Body: "comment"})
					return err
				}
			case "children":
				securityExec(t, s.db, `INSERT INTO kanban_card(title,parent_id) SELECT 'child',1 FROM generate_series(1,99)`)
				s.cards.repo = &securityCardBarrier{CardRepositoryInterface: s.cards.repo, children: barrier}
				wantCode, wantCount = apperr.CodeValidation, 100
				countSQL = "SELECT count(*) FROM kanban_card WHERE parent_id=1 AND deleted_at IS NULL"
				create = func() error {
					_, err := s.cards.CreateCard(ctx, dto.CreateCardRequest{Title: "child", ParentID: &one})
					return err
				}
			case "cards":
				securityExec(t, s.db, `INSERT INTO kanban_card(title,column_id) SELECT 'card',1 FROM generate_series(1,298)`)
				s.cards.repo = &securityCardBarrier{CardRepositoryInterface: s.cards.repo, active: barrier}
				wantCode, wantCount = apperr.CodeBoardCardLimitReached, 300
				countSQL = "SELECT count(*) FROM kanban_card WHERE column_id=1 AND parent_id IS NULL AND NOT is_archived AND deleted_at IS NULL"
				create = func() error {
					_, err := s.cards.CreateCard(ctx, dto.CreateCardRequest{Title: "card", ColumnID: &one})
					return err
				}
			}
			errs := securityInterleave(t, s.db, barrier, create, create)
			success := 0
			for _, err := range errs {
				if err == nil {
					success++
					continue
				}
				var domainErr *apperr.Error
				if !errors.As(err, &domainErr) || domainErr.Code != wantCode {
					t.Fatalf("expected limit error %s, got %v", wantCode, err)
				}
				if kind == "children" && domainErr.Message != "maximum number of subtasks (100) per card reached" {
					t.Fatalf("expected subtask limit, got %v", err)
				}
			}
			if success != 1 {
				t.Fatalf("success=%d errors=%v", success, errs)
			}
			var count int
			if err := s.db.QueryRow(ctx, countSQL).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != wantCount {
				t.Fatalf("count=%d, want %d", count, wantCount)
			}
		})
	}
}

func TestSecurityR11LegacyOverflowDoesNotBlockUnrelatedUndo(t *testing.T) {
	s := securityServicesDB(t)
	ctx := securityUser(1)
	title := "renamed"
	if _, err := s.cards.UpdateCard(ctx, 1, dto.UpdateCardRequest{Title: &title}); err != nil {
		t.Fatal(err)
	}
	securityExec(t, s.db, `INSERT INTO kanban_card(id,title,column_id) VALUES(2,'legacy',1);
	INSERT INTO kanban_attachment(filename,storage_key,content_type,size_bytes,card_id,context)
	SELECT 'f','k','text/plain',1,2,'info' FROM generate_series(1,17)`)
	if _, _, _, err := s.history.Undo(ctx, 1); err != nil {
		t.Fatalf("legacy overflow in another card blocked title undo: %v", err)
	}
	card, err := s.cards.repo.GetCardRow(ctx, 1)
	if err != nil || card == nil || card.Title != "old" {
		t.Fatalf("title was not restored: card=%+v err=%v", card, err)
	}
	var count int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM kanban_attachment WHERE card_id=2 AND deleted_at IS NULL").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 17 {
		t.Fatalf("unrelated attachments changed: count=%d", count)
	}
}

func TestSecurityR11ArchiveRestoreAtBoardLimit(t *testing.T) {
	s := securityServicesDB(t)
	ctx := securityUser(1)
	securityExec(t, s.db, `UPDATE kanban_card SET is_archived=true WHERE id=1; INSERT INTO kanban_card(title,column_id) SELECT 'card',1 FROM generate_series(1,300)`)
	if _, err := s.cards.ArchiveCard(ctx, 1); err == nil {
		t.Fatal("unarchive exceeded board limit")
	}
}

func TestSecurityR11UndoRestoreRespectsLimits(t *testing.T) {
	for _, kind := range []string{"attachment", "comment", "card", "child"} {
		t.Run(kind, func(t *testing.T) {
			s := securityServicesDB(t)
			ctx := securityUser(1)
			switch kind {
			case "attachment":
				a, err := s.attachments.CreateAttachment(ctx, 1, dto.CreateAttachmentRequest{Filename: "f", StorageKey: "k", ContentType: "text/plain", Context: "info", SizeBytes: 1})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.attachments.DeleteAttachment(ctx, a); err != nil {
					t.Fatal(err)
				}
				securityExec(t, s.db, `INSERT INTO kanban_attachment(filename,storage_key,content_type,size_bytes,card_id,context) SELECT 'f','k','text/plain',1,1,'info' FROM generate_series(1,16)`)
			case "comment":
				c, err := s.comments.CreateComment(ctx, 1, dto.CreateCommentRequest{Body: "comment"})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.comments.DeleteComment(ctx, 1, c.ID); err != nil {
					t.Fatal(err)
				}
				securityExec(t, s.db, `INSERT INTO kanban_card_comment(body,card_id,author_id) SELECT 'c',1,1 FROM generate_series(1,300)`)
			case "card":
				if err := s.cards.DeleteCard(ctx, 1); err != nil {
					t.Fatal(err)
				}
				securityExec(t, s.db, `INSERT INTO kanban_card(title,column_id) SELECT 'card',1 FROM generate_series(1,300)`)
			case "child":
				one := int64(1)
				child, err := s.cards.CreateCard(ctx, dto.CreateCardRequest{Title: "child", ParentID: &one})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.cards.DeleteCard(ctx, child.ID); err != nil {
					t.Fatal(err)
				}
				securityExec(t, s.db, `INSERT INTO kanban_card(title,parent_id) SELECT 'child',1 FROM generate_series(1,100)`)
			}
			if _, _, _, err := s.history.Undo(ctx, 1); err == nil {
				t.Fatal("undo exceeded limit")
			}
		})
	}
}

func TestSecurityR12ProjectCreationRollsBackAndCanRetry(t *testing.T) {
	s := securityServicesDB(t)
	ctx := securityUser(1)
	securityExec(t, s.db, `CREATE FUNCTION reject_board() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test board failure'; END $$; CREATE TRIGGER reject_board BEFORE INSERT ON kanban_board FOR EACH ROW EXECUTE FUNCTION reject_board();`)
	if _, err := s.projects.CreateProject(ctx, dto.CreateProjectRequest{Name: "new project"}); err == nil {
		t.Fatal("board failure accepted")
	}
	var count int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM kanban_project").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("partial projects=%d", count)
	}
	securityExec(t, s.db, "DROP TRIGGER reject_board ON kanban_board")
	if _, err := s.projects.CreateProject(ctx, dto.CreateProjectRequest{Name: "new project"}); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM kanban_project").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("projects=%d", count)
	}
}
