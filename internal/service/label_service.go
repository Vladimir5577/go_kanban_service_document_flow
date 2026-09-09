package service

import (
	"context"
	"encoding/json"
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
	cardRepo          repository.CardRepositoryInterface
	columnRepo        repository.ColumnRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	History           HistoryLogger
}

func NewLabelService(
	repo repository.LabelRepositoryInterface,
	permSvc *PermissionService,
	boardRepo repository.BoardRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
) *LabelService {
	return &LabelService{
		repo:              repo,
		permSvc:           permSvc,
		boardRepo:         boardRepo,
		cardRepo:          cardRepo,
		columnRepo:        columnRepo,
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
			ProjectID:  projectID,
			Action:      "label.created",
			EntityTitle: created.Name,
			EntityLink:  historyBoardPath(projectID, boardID),
			EntityKeys:  historyKeys("label", created.ID),
			Undo:       []model.UndoStep{{Op: "label.delete", LabelID: created.ID}},
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
	snap, _ := json.Marshal(map[string]any{
		"id": label.ID, "name": label.Name, "color": label.Color, "boardId": label.BoardID, "cardIds": cardIDs,
	})
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  projectID,
		Action:      "label.deleted",
		EntityTitle: label.Name,
		EntityLink:  historyBoardPath(projectID, boardID),
		EntityKeys:  append(historyKeys("label", labelID), historyKeys("card", cardIDs...)...),
		Undo:       []model.UndoStep{{Op: "label.insert", LabelID: labelID, Snapshot: snap}},
	})
	return nil
}

func (s *LabelService) ToggleLabel(ctx context.Context, projectID int64, boardID int64, cardID int64, labelID int64) (string, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return "", err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return "", err
	}
	card, err := s.ensureCardInBoard(ctx, boardID, cardID)
	if err != nil {
		return "", err
	}

	label, err := s.getLabelInBoard(ctx, boardID, labelID)
	if err != nil {
		return "", err
	}

	added, err := s.repo.ToggleLabel(ctx, cardID, labelID)
	if err != nil {
		return "", err
	}
	on := !added
	action := "label.removed"
	if added {
		action = "label.added"
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  projectID,
		Action:      action,
		EntityTitle: label.Name,
		EntityLink:  historyTaskPath(projectID, boardID, cardID),
		After:       card.Title,
		EntityKeys:  []string{HistoryKey("card", cardID), HistoryKey("label", labelID)},
		Undo:       []model.UndoStep{{Op: "label.toggle", CardID: cardID, LabelID: labelID, On: &on}},
	})

	if added {
		s.publishLabelsPatch(ctx, cardID)
		return "attached", nil
	}
	s.publishLabelsPatch(ctx, cardID)
	return "detached", nil
}

func (s *LabelService) publishLabelsPatch(ctx context.Context, cardID int64) {
	if s.realtimePublisher == nil {
		return
	}
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		patch, err := s.realtimePublisher.BuildLabels(ctx, cardID)
		if err != nil {
			return err
		}
		return s.realtimePublisher.PublishCardPatchByID(ctx, cardID, patch, realtimeSenderID(ctx))
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

func (s *LabelService) ensureCardInBoard(ctx context.Context, boardID int64, cardID int64) (*model.Card, error) {
	card, err := s.cardRepo.GetCard(ctx, cardID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeCardNotFound)
	}
	column, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}
	if column.BoardID != boardID {
		return nil, apperr.New(apperr.CodeCardNotFound, "card not found")
	}
	return card, nil
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

