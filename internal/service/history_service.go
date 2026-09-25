package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const undoMaxAge = time.Hour

type HistoryServiceInterface interface {
	Append(ctx context.Context, e model.HistoryWrite) error
	List(ctx context.Context, projectID, cursor int64, limit int32, userID int64, title string) (*model.HistoryListPage, error)
	ListByCard(ctx context.Context, cardID, cursor int64, limit int32, userID int64, title string) (*model.HistoryListPage, error)
	Undo(ctx context.Context, projectID int64) (action, entityTitle string, isChild bool, err error)
}

type HistoryService struct {
	repo       repository.HistoryRepositoryInterface
	memberRepo repository.ProjectMemberRepositoryInterface
	permSvc    *PermissionService
	realtime   *KanbanRealtimePublisher
	cardRepo   repository.CardRepositoryInterface
	columnRepo repository.ColumnRepositoryInterface
}

func NewHistoryService(
	repo repository.HistoryRepositoryInterface,
	memberRepo repository.ProjectMemberRepositoryInterface,
	permSvc *PermissionService,
	realtime *KanbanRealtimePublisher,
	cardRepo repository.CardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
) *HistoryService {
	return &HistoryService{
		repo:       repo,
		memberRepo: memberRepo,
		permSvc:    permSvc,
		realtime:   realtime,
		cardRepo:   cardRepo,
		columnRepo: columnRepo,
	}
}

func (s *HistoryService) Append(ctx context.Context, e model.HistoryWrite) error {
	if e.EntityLink == "" {
		e.EntityLink = s.historyEntityLink(ctx, e)
	}
	return s.repo.Append(ctx, currentUserID(ctx), e)
}

func (s *HistoryService) historyEntityLink(ctx context.Context, e model.HistoryWrite) string {
	cardID := e.CardID
	if cardID == 0 && e.EntityType == "card" {
		cardID = e.EntityID
	}
	if s.cardRepo != nil && s.columnRepo != nil && cardID != 0 {
		if card, err := s.cardRepo.GetCard(ctx, cardID); err == nil && card != nil {
			if col, err := s.columnRepo.GetColumn(ctx, card.ColumnID); err == nil && col != nil {
				if e.Action == "card.archived" {
					return historyArchivePath(e.ProjectID, col.BoardID)
				}
				return historyTaskPath(e.ProjectID, col.BoardID, cardID)
			}
		}
	}
	if e.EntityType == "column" && s.columnRepo != nil {
		if col, err := s.columnRepo.GetColumn(ctx, e.EntityID); err == nil && col != nil {
			return historyBoardPath(e.ProjectID, col.BoardID)
		}
	}
	if e.EntityType == "board" {
		return historyBoardPath(e.ProjectID, e.EntityID)
	}
	if e.EntityType == "project" && (strings.HasPrefix(e.Action, "member") || e.Action == "members.replaced") {
		return historyEditPath(e.ProjectID)
	}
	return historyProjectPath(e.ProjectID)
}

func (s *HistoryService) List(ctx context.Context, projectID, cursor int64, limit int32, userID int64, title string) (*model.HistoryListPage, error) {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.list(ctx, projectID, 0, cursor, limit, userID, title)
}

func (s *HistoryService) ListByCard(ctx context.Context, cardID, cursor int64, limit int32, userID int64, title string) (*model.HistoryListPage, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.list(ctx, projectID, cardID, cursor, limit, userID, title)
}

func (s *HistoryService) list(ctx context.Context, projectID, cardID, cursor int64, limit int32, userID int64, title string) (*model.HistoryListPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.repo.List(ctx, projectID, cardID, userID, cursor, likeContains(title), limit+1)
	if err != nil {
		return nil, err
	}
	page := &model.HistoryListPage{}
	if int32(len(rows)) > limit {
		page.HasMore = true
		rows = rows[:limit]
	}
	page.Items = rows
	if len(rows) > 0 {
		page.NextCursor = rows[len(rows)-1].ID
	}
	return page, nil
}

func (s *HistoryService) Undo(ctx context.Context, projectID int64) (string, string, bool, error) {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return "", "", false, err
	}
	user, ok := middleware.GetUser(ctx)
	if !ok || user.ID == 0 {
		return "", "", false, apperr.ErrUnauthorized
	}

	entry, err := s.repo.LastByUser(ctx, projectID, user.ID)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return "", "", false, apperr.New(apperr.CodeUndoEmpty, "нечего отменять")
		}
		return "", "", false, err
	}
	if entry.CreatedAt.IsZero() || time.Since(entry.CreatedAt) > undoMaxAge {
		return "", "", false, apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}

	overlap, err := s.repo.HasForeignOverlap(ctx, projectID, entry.ID, user.ID, entry.EntityType, entry.EntityID, undoNested(entry.Action))
	if err != nil {
		return "", "", false, err
	}
	if overlap {
		return "", "", false, apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}
	if entry.Action == "column.created" && s.columnRepo != nil {
		hasCards, err := s.columnRepo.HasCardsByColumn(ctx, entry.EntityID)
		if err != nil {
			return "", "", false, err
		}
		if hasCards {
			return "", "", false, apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
		}
	}

	if isMembersHistoryAction(entry.Action) {
		if err := s.applyMembersUndo(ctx, entry); err != nil {
			return "", "", false, err
		}
		if err := s.repo.DeleteEntry(ctx, entry.ID); err != nil {
			return "", "", false, err
		}
	} else {
		if err := s.repo.ApplyUndo(ctx, *entry); err != nil {
			return "", "", false, err
		}
	}

	s.publishUndo(ctx, *entry)
	return entry.Action, entry.EntityTitle, model.HistoryIsChild(entry.EntityType, entry.EntityID, entry.CardID), nil
}

func isMembersHistoryAction(action string) bool {
	return action == "members.replaced" || action == "member.role" || action == "member.removed"
}

// undoNested — откат создания сносит контейнер вместе с вложенным.
// Чужая история детей, комментариев, вложений и карточек внутри тоже запрещает откат.
func undoNested(action string) bool {
	switch action {
	case "card.created", "card.duplicated", "column.created", "board.created", "project.created":
		return true
	default:
		return false
	}
}

func (s *HistoryService) applyMembersUndo(ctx context.Context, entry *model.HistoryUndoEntry) error {
	if entry.Before == "" {
		return apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}
	var rows []struct {
		UserID   int64   `json:"userId"`
		Role     string  `json:"role"`
		FolderID *int64  `json:"folderId"`
		Position float64 `json:"position"`
	}
	if err := json.Unmarshal([]byte(entry.Before), &rows); err != nil {
		return apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}
	members := make([]model.ProjectUser, 0, len(rows))
	for _, row := range rows {
		members = append(members, model.ProjectUser{
			KanbanProjectID: entry.ProjectID,
			UserID:          row.UserID,
			Role:            row.Role,
			FolderID:        row.FolderID,
			Position:        row.Position,
		})
	}
	return s.memberRepo.ReplaceMembers(ctx, entry.ProjectID, members)
}

func (s *HistoryService) publishUndo(ctx context.Context, entry model.HistoryUndoEntry) {
	if s.realtime == nil || entry.CardID == 0 || s.cardRepo == nil {
		return
	}
	cardID := entry.CardID
	s.realtime.TryPublish(ctx, func(ctx context.Context) error {
		card, err := s.cardRepo.GetCard(ctx, cardID)
		if err != nil {
			return s.realtime.PublishCardDeleted(ctx, 0, cardID, realtimeSenderID(ctx))
		}
		return s.realtime.PublishCardPatch(ctx, card, map[string]any{
			"id":         card.ID,
			"title":      card.Title,
			"position":   card.Position,
			"columnId":   card.ColumnID,
			"isArchived": card.IsArchived,
		}, realtimeSenderID(ctx))
	})
}

func likeContains(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `%`, `\%`)
	q = strings.ReplaceAll(q, `_`, `\_`)
	return q
}
