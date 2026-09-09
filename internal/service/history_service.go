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
	Undo(ctx context.Context, projectID int64) (action, entityTitle string, err error)
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
	if s.cardRepo != nil && s.columnRepo != nil {
		if cardID := firstHistoryID(e.EntityKeys, "card"); cardID != 0 {
			if card, err := s.cardRepo.GetCard(ctx, cardID); err == nil && card != nil {
				if col, err := s.columnRepo.GetColumn(ctx, card.ColumnID); err == nil && col != nil {
					if e.Action == "card.archived" {
						return historyArchivePath(e.ProjectID, col.BoardID)
					}
					return historyTaskPath(e.ProjectID, col.BoardID, cardID)
				}
			}
		}
		if columnID := firstHistoryID(e.EntityKeys, "column"); columnID != 0 {
			if col, err := s.columnRepo.GetColumn(ctx, columnID); err == nil && col != nil {
				return historyBoardPath(e.ProjectID, col.BoardID)
			}
		}
	}
	if boardID := firstHistoryID(e.EntityKeys, "board"); boardID != 0 {
		return historyBoardPath(e.ProjectID, boardID)
	}
	if strings.HasPrefix(e.Action, "member") || e.Action == "members.replaced" {
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

func (s *HistoryService) Undo(ctx context.Context, projectID int64) (string, string, error) {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return "", "", err
	}
	user, ok := middleware.GetUser(ctx)
	if !ok || user.ID == 0 {
		return "", "", apperr.ErrUnauthorized
	}

	entry, err := s.repo.LastByUser(ctx, projectID, user.ID)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return "", "", apperr.New(apperr.CodeUndoEmpty, "нечего отменять")
		}
		return "", "", err
	}
	if !entry.CreatedAt.Valid || time.Since(entry.CreatedAt.Time) > undoMaxAge {
		return "", "", apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}

	overlap, err := s.repo.HasForeignOverlap(ctx, projectID, entry.ID, user.ID, entry.EntityKeys)
	if err != nil {
		return "", "", err
	}
	if overlap {
		return "", "", apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}

	var payload model.HistoryPayload
	if err := json.Unmarshal(entry.Payload, &payload); err != nil {
		return "", "", err
	}
	if len(payload.Steps) == 0 {
		return "", "", apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
	}

	var rest []model.UndoStep
	for _, step := range payload.Steps {
		if step.Op == "members.replace" {
			if err := s.applyMembersReplace(ctx, step); err != nil {
				return "", "", err
			}
			continue
		}
		rest = append(rest, step)
	}
	if err := s.repo.ApplyUndo(ctx, entry.ID, rest); err != nil {
		return "", "", err
	}

	s.publishUndo(ctx, payload.Steps)
	return entry.Action, entry.EntityTitle, nil
}

func (s *HistoryService) applyMembersReplace(ctx context.Context, step model.UndoStep) error {
	var rows []struct {
		UserID   int64   `json:"userId"`
		Role     string  `json:"role"`
		FolderID *int64  `json:"folderId"`
		Position float64 `json:"position"`
	}
	if err := json.Unmarshal(step.Snapshot, &rows); err != nil {
		return err
	}
	members := make([]model.ProjectUser, 0, len(rows))
	for _, row := range rows {
		members = append(members, model.ProjectUser{
			KanbanProjectID: step.ProjectID,
			UserID:          row.UserID,
			Role:            row.Role,
			FolderID:        row.FolderID,
			Position:        row.Position,
		})
	}
	return s.memberRepo.ReplaceMembers(ctx, step.ProjectID, members)
}

func (s *HistoryService) publishUndo(ctx context.Context, steps []model.UndoStep) {
	if s.realtime == nil {
		return
	}
	seen := map[int64]struct{}{}
	for _, step := range steps {
		if step.CardID == 0 {
			continue
		}
		if _, ok := seen[step.CardID]; ok {
			continue
		}
		seen[step.CardID] = struct{}{}
		cardID := step.CardID
		s.realtime.TryPublish(ctx, func(ctx context.Context) error {
			card, err := s.cardRepo.GetCard(ctx, cardID)
			if err != nil {
				colID := step.ColumnID
				if colID == 0 && s.columnRepo != nil {
					return s.realtime.PublishCardDeleted(ctx, 0, cardID, realtimeSenderID(ctx))
				}
				if s.columnRepo == nil {
					return nil
				}
				col, colErr := s.columnRepo.GetColumn(ctx, colID)
				if colErr != nil {
					return nil
				}
				return s.realtime.PublishCardDeleted(ctx, col.BoardID, cardID, realtimeSenderID(ctx))
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
