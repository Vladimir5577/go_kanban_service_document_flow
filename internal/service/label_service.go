package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const defaultLabelColor = "bg-primary"

var allowedLabelColors = map[string]struct{}{
	"bg-dark":       {},
	"bg-coral":      {},
	"bg-bronze":     {},
	"bg-lemon":      {},
	"bg-olive":      {},
	"bg-sea":        {},
	"bg-periwinkle": {},
	"bg-lilac":      {},
	"bg-gray":       {},
	"bg-danger":     {},
	"bg-orange":     {},
	"bg-warning":    {},
	"bg-success":    {},
	"bg-info":       {},
	"bg-primary":    {},
	"bg-magenta":    {},
}

type LabelServiceInterface interface {
	GetLabels(ctx context.Context, projectID int64, boardID int64) ([]model.Label, error)
	CreateLabel(ctx context.Context, projectID int64, boardID int64, req dto.CreateLabelRequest) (*model.Label, error)
	DeleteLabel(ctx context.Context, projectID int64, boardID int64, labelID int64) error
	ToggleLabel(ctx context.Context, projectID int64, boardID int64, cardID int64, labelID int64) (string, error)
}

type LabelService struct {
	repo              repository.LabelRepositoryInterface
	permSvc           *PermissionService
	boardRepo         repository.BoardRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	History           HistoryLogger
}

func NewLabelService(
	repo repository.LabelRepositoryInterface,
	permSvc *PermissionService,
	boardRepo repository.BoardRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
) *LabelService {
	return &LabelService{
		repo:              repo,
		permSvc:           permSvc,
		boardRepo:         boardRepo,
		realtimePublisher: realtimePublisher,
	}
}

func (s *LabelService) GetLabels(ctx context.Context, projectID int64, boardID int64) ([]model.Label, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetLabels(ctx, boardID)
}

func (s *LabelService) CreateLabel(ctx context.Context, projectID int64, boardID int64, req dto.CreateLabelRequest) (*model.Label, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	name, err := normalizeLabelName(req.Name)
	if err != nil {
		return nil, err
	}

	l := &model.Label{
		Name:  name,
		Color: normalizeLabelColor(req.Color),
	}
	created, err := s.repo.CreateLabel(ctx, boardID, l)
	if err == nil && created != nil {
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "label.created",
			EntityType:  "label",
			EntityID:    created.ID,
			EntityTitle: created.Name,
			EntityLink:  historyBoardPath(projectID, boardID),
		})
	}
	return created, err
}

func (s *LabelService) DeleteLabel(ctx context.Context, projectID int64, boardID int64, labelID int64) error {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	label, err := s.getLabelInBoard(ctx, boardID, labelID)
	if err != nil {
		return err
	}
	cardIDs, _ := s.repo.GetCardIDsByLabel(ctx, labelID)
	if err := s.repo.DeleteLabel(ctx, labelID); err != nil {
		return err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   projectID,
		Action:      "label.deleted",
		EntityType:  "label",
		EntityID:    labelID,
		EntityTitle: label.Name,
		EntityLink:  historyBoardPath(projectID, boardID),
		// before = сколько карточек имели метку; id не пишем, undo = RestoreLabel.
		Before: historyLabelCardsCount(len(cardIDs)),
	})
	return nil
}

func (s *LabelService) ToggleLabel(ctx context.Context, projectID int64, boardID int64, cardID int64, labelID int64) (string, error) {
	// Права, проект, доска и «не подзадача» — одним уже готовым запросом
	// контекста карточки, без отдельных чтений доски, карточки и колонки.
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleEditor)
	if err != nil {
		return "", err
	}
	if acc.ProjectID != projectID || acc.BoardID != boardID {
		return "", apperr.New(apperr.CodeCardNotFound, "card not found")
	}

	label, err := s.getLabelInBoard(ctx, boardID, labelID)
	if err != nil {
		return "", err
	}

	added, labelIDs, err := s.repo.ToggleLabel(ctx, cardID, labelID)
	if err != nil {
		return "", err
	}
	action := "label.removed"
	if added {
		action = "label.added"
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   projectID,
		Action:      action,
		EntityType:  "label",
		EntityID:    labelID,
		CardID:      cardID,
		EntityTitle: label.Name,
		EntityLink:  historyTaskPath(projectID, boardID, cardID),
	})

	s.publishLabelsPatch(ctx, boardID, cardID, labelIDs)
	if added {
		return "attached", nil
	}
	return "detached", nil
}

// publishLabelsPatch шлёт новый набор меток карточки. Доска и набор уже
// известны, поэтому из базы нужны только метки доски — ради имён и цветов.
func (s *LabelService) publishLabelsPatch(ctx context.Context, boardID, cardID int64, labelIDs []int64) {
	if s.realtimePublisher == nil {
		return
	}
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		patch, err := s.realtimePublisher.buildCardLabels(ctx, labelIDs, boardID)
		if err != nil {
			return err
		}
		patch["id"] = cardID
		return s.realtimePublisher.PublishCardUpdated(ctx, boardID, patch, realtimeSenderID(ctx))
	})
}

func (s *LabelService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.boardRepo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeBoardNotFound)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.New(apperr.CodeBoardNotFound, "board not found")
	}
	return board, nil
}

func (s *LabelService) getLabelInBoard(ctx context.Context, boardID int64, labelID int64) (*model.Label, error) {
	label, err := s.repo.GetLabel(ctx, labelID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeLabelNotFound)
	}
	if label.BoardID != boardID {
		return nil, apperr.New(apperr.CodeLabelNotFound, "label not found")
	}
	return label, nil
}

func normalizeLabelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperr.New(apperr.CodeLabelNameRequired, "label name required")
	}
	return name, nil
}

func normalizeLabelColor(color string) string {
	color = strings.TrimSpace(color)
	if _, ok := allowedLabelColors[color]; ok {
		return color
	}
	return defaultLabelColor
}

func mapNoRowsToNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrNotFound
	}
	return err
}

