package service

import (
	"context"
	"strconv"
	"time"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

// Потолок от опечатки: 10000 часов. Сутки в строке на фронте — 8 часов, здесь только минуты.
const maxTimeMinutes = 10000 * 60

func validTimeMinutes(n int) error {
	if n <= 0 || n > maxTimeMinutes {
		return apperr.New(apperr.CodeValidation, "invalid time minutes")
	}
	return nil
}

// canSetEstimate — план меняет автор задачи, редактор или админ. Владелец уже приходит как админ.
func canSetEstimate(userID int64, createdBy *int64, role Role) bool {
	if hasRole(role, RoleEditor) {
		return true
	}
	return createdBy != nil && *createdBy == userID
}

type TimeServiceInterface interface {
	SetEstimate(ctx context.Context, cardID int64, req dto.SetEstimateRequest) (*dto.EstimateResponse, error)
	CreateEntry(ctx context.Context, cardID int64, req dto.CreateTimeEntryRequest) (*dto.TimeEntryResponse, error)
	DeleteEntry(ctx context.Context, cardID, entryID int64) (*dto.DeleteTimeEntryResponse, error)
}

type TimeService struct {
	repo              repository.CardRepositoryInterface
	permSvc           *PermissionService
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	History           HistoryLogger
}

func NewTimeService(repo repository.CardRepositoryInterface, permSvc *PermissionService, realtime *KanbanRealtimePublisher, notificationSvc *KanbanNotificationService) *TimeService {
	return &TimeService{repo: repo, permSvc: permSvc, realtimePublisher: realtime, notificationSvc: notificationSvc}
}

func (s *TimeService) SetEstimate(ctx context.Context, cardID int64, req dto.SetEstimateRequest) (*dto.EstimateResponse, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	card, err := s.repo.GetCardRow(ctx, cardID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	if !canSetEstimate(user.ID, card.CreatedByID, acc.Role) {
		return nil, accessDenied()
	}
	if req.EstimateMinutes != nil {
		if err := validTimeMinutes(*req.EstimateMinutes); err != nil {
			return nil, err
		}
	}
	if sameEstimate(card.EstimateMinutes, req.EstimateMinutes) {
		return estimateResponse(card), nil
	}

	updated, err := s.repo.SetCardEstimate(ctx, cardID, req.EstimateMinutes)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  acc.ProjectID,
		Action:     "card.updated.estimate",
		EntityType: "card",
		EntityID:   cardID,
		CardID:     cardID,
		EntityLink: historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
		Before:     minutesText(card.EstimateMinutes),
		After:      minutesText(updated.EstimateMinutes),
	})
	s.publishTime(ctx, acc.BoardID, cardID, map[string]any{
		"estimateMinutes": updated.EstimateMinutes,
		"updatedAt":       formatRealtimeTimeValue(updated.UpdatedAt),
	})
	return estimateResponse(updated), nil
}

func (s *TimeService) CreateEntry(ctx context.Context, cardID int64, req dto.CreateTimeEntryRequest) (*dto.TimeEntryResponse, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	if err := validTimeMinutes(req.Minutes); err != nil {
		return nil, err
	}
	workAt := time.Now()
	if req.WorkAt != nil && !req.WorkAt.IsZero() {
		workAt = *req.WorkAt
	}

	entry, err := s.repo.CreateTimeEntry(ctx, cardID, user.ID, req.Minutes, workAt)
	if err != nil {
		return nil, err
	}
	logged, err := s.loggedMinutes(ctx, cardID)
	if err != nil {
		return nil, err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "time_entry.created",
		EntityType:  "time_entry",
		EntityID:    entry.ID,
		CardID:      cardID,
		EntityTitle: entry.TrackedAt.Format(time.RFC3339),
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
		After:       strconv.Itoa(entry.Minutes),
	})
	s.publishTime(ctx, acc.BoardID, cardID, map[string]any{"loggedMinutes": logged})
	if s.notificationSvc != nil {
		actorID := user.ID
		runDetached(ctx, notifyTimeout, "failed to notify kanban time tracked", func(ctx context.Context) error {
			s.notificationSvc.NotifyTimeTracked(ctx, acc.ProjectID, acc.BoardID, acc.BoardTitle, cardID, actorID, acc.CardTitle)
			return nil
		})
	}
	return &dto.TimeEntryResponse{
		ID:            entry.ID,
		Minutes:       entry.Minutes,
		WorkAt:        entry.TrackedAt,
		LoggedMinutes: logged,
	}, nil
}

func (s *TimeService) DeleteEntry(ctx context.Context, cardID, entryID int64) (*dto.DeleteTimeEntryResponse, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	entry, err := s.repo.GetTimeEntry(ctx, entryID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeTimeEntryNotFound)
	}
	if entry.CardID != cardID {
		return nil, apperr.New(apperr.CodeTimeEntryNotFound, "time entry not found")
	}
	if entry.UserID != user.ID {
		return nil, apperr.New(apperr.CodeTimeEntryAuthorOnly, "time entry author only")
	}
	if err := s.repo.DeleteTimeEntry(ctx, entryID); err != nil {
		return nil, withNotFoundCode(err, apperr.CodeTimeEntryNotFound)
	}
	logged, err := s.loggedMinutes(ctx, cardID)
	if err != nil {
		return nil, err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "time_entry.deleted",
		EntityType:  "time_entry",
		EntityID:    entry.ID,
		CardID:      cardID,
		EntityTitle: entry.TrackedAt.Format(time.RFC3339),
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
		After:       strconv.Itoa(entry.Minutes),
	})
	s.publishTime(ctx, acc.BoardID, cardID, map[string]any{"loggedMinutes": logged})
	return &dto.DeleteTimeEntryResponse{LoggedMinutes: logged}, nil
}

func (s *TimeService) loggedMinutes(ctx context.Context, cardID int64) (int, error) {
	sums, err := s.repo.SumLoggedMinutes(ctx, []int64{cardID})
	if err != nil {
		return 0, err
	}
	return sums[cardID], nil
}

func (s *TimeService) publishTime(ctx context.Context, boardID, cardID int64, patch map[string]any) {
	if s.realtimePublisher == nil {
		return
	}
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		patch["id"] = cardID
		return s.realtimePublisher.PublishCardUpdated(ctx, boardID, patch, realtimeSenderID(ctx))
	})
}

func estimateResponse(card *model.Card) *dto.EstimateResponse {
	return &dto.EstimateResponse{
		ID:              card.ID,
		EstimateMinutes: card.EstimateMinutes,
		UpdatedAt:       card.UpdatedAt,
	}
}

func sameEstimate(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func minutesText(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}
