package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

// Pause the first operation after its read. The second must either finish on
// the old, unprotected implementation or demonstrably wait on the first's lock.
type securityBarrier struct {
	db      *pgxpool.Pool
	once    sync.Once
	opened  sync.Once
	entered chan int32
	release chan struct{}
}

func newSecurityBarrier(db *pgxpool.Pool) *securityBarrier {
	return &securityBarrier{db: db, entered: make(chan int32, 1), release: make(chan struct{})}
}

func (b *securityBarrier) pause(ctx context.Context) error {
	if b == nil {
		return nil
	}
	first := false
	b.once.Do(func() { first = true })
	if !first {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var pid int32
	if err := repository.Connection(ctx, b.db).QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		return err
	}
	b.entered <- pid
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *securityBarrier) resume() {
	b.opened.Do(func() { close(b.release) })
}

func securityInterleave(t *testing.T, db *pgxpool.Pool, barrier *securityBarrier, a, b func() error) []error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer barrier.resume()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- a() }()
	var pid int32
	select {
	case pid = <-barrier.entered:
	case err := <-first:
		t.Fatalf("first operation did not reach the read barrier: %v", err)
	case <-ctx.Done():
		t.Fatal("first operation timed out before the read barrier")
	}
	go func() { second <- b() }()
	errs := make([]error, 2)
	secondFinished := false
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case errs[1] = <-second:
			secondFinished, ready = true, true
		case <-ticker.C:
			var blocked bool
			if err := db.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity WHERE $1::integer = ANY(pg_blocking_pids(pid))
			)`, pid).Scan(&blocked); err != nil {
				t.Fatalf("observe second operation waiting for first: %v", err)
			}
			ready = blocked
		case <-ctx.Done():
			t.Fatal("second operation neither finished nor waited on the first operation's lock")
		}
	}
	barrier.resume()
	select {
	case errs[0] = <-first:
	case <-ctx.Done():
		t.Fatal("first operation did not finish after releasing its read barrier")
	}
	if !secondFinished {
		select {
		case errs[1] = <-second:
		case <-ctx.Done():
			t.Fatal("second operation did not finish after releasing the first")
		}
	}
	return errs
}

type securityHistoryBarrier struct {
	repository.HistoryRepositoryInterface
	barrier *securityBarrier
}

func (r *securityHistoryBarrier) HasForeignOverlap(ctx context.Context, projectID, afterID, userID int64, entityType string, entityID int64, includeNested bool) (bool, error) {
	overlap, err := r.HistoryRepositoryInterface.HasForeignOverlap(ctx, projectID, afterID, userID, entityType, entityID, includeNested)
	if err == nil {
		err = r.barrier.pause(ctx)
	}
	return overlap, err
}

type securityCardBarrier struct {
	repository.CardRepositoryInterface
	row, children, active *securityBarrier
}

func (r *securityCardBarrier) GetCardRow(ctx context.Context, id int64) (*model.Card, error) {
	card, err := r.CardRepositoryInterface.GetCardRow(ctx, id)
	if err == nil {
		err = r.row.pause(ctx)
	}
	return card, err
}

func (r *securityCardBarrier) GetChildStats(ctx context.Context, parentID int64) (int64, float64, error) {
	count, position, err := r.CardRepositoryInterface.GetChildStats(ctx, parentID)
	if err == nil {
		err = r.children.pause(ctx)
	}
	return count, position, err
}

func (r *securityCardBarrier) CountActiveCardsByBoard(ctx context.Context, boardID int64) (int, error) {
	count, err := r.CardRepositoryInterface.CountActiveCardsByBoard(ctx, boardID)
	if err == nil {
		err = r.active.pause(ctx)
	}
	return count, err
}

type securityColumnBarrier struct {
	repository.ColumnRepositoryInterface
	barrier *securityBarrier
}

func (r *securityColumnBarrier) GetColumn(ctx context.Context, id int64) (*model.Column, error) {
	column, err := r.ColumnRepositoryInterface.GetColumn(ctx, id)
	if err == nil {
		err = r.barrier.pause(ctx)
	}
	return column, err
}

type securityAttachmentBarrier struct {
	repository.AttachmentRepositoryInterface
	barrier *securityBarrier
}

func (r *securityAttachmentBarrier) GetAttachmentsByCard(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error) {
	attachments, err := r.AttachmentRepositoryInterface.GetAttachmentsByCard(ctx, cardID, contextStr)
	if err == nil {
		err = r.barrier.pause(ctx)
	}
	return attachments, err
}

type securityCommentBarrier struct {
	repository.CommentRepositoryInterface
	barrier *securityBarrier
}

func (r *securityCommentBarrier) GetCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error) {
	counts, err := r.CommentRepositoryInterface.GetCountsByCardIDs(ctx, cardIDs)
	if err == nil {
		err = r.barrier.pause(ctx)
	}
	return counts, err
}

type securityProjectMemberBarrier struct {
	repository.ProjectMemberRepositoryInterface
	barrier *securityBarrier
}

func (r *securityProjectMemberBarrier) UpdateProjectPlacement(ctx context.Context, projectID, userID int64, folderID *int64, position float64) (*model.ProjectUser, error) {
	member, err := r.ProjectMemberRepositoryInterface.UpdateProjectPlacement(ctx, projectID, userID, folderID, position)
	if err == nil {
		err = r.barrier.pause(ctx)
	}
	return member, err
}
