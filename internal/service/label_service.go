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
	"bg-primary": {},
	"bg-warning": {},
	"bg-success": {},
	"bg-danger":  {},
	"bg-info":    {},
	"bg-dark":    {},
}

type LabelServiceInterface interface {
	GetLabels(ctx context.Context, projectID int64, boardID int64) ([]model.Label, error)
	CreateLabel(ctx context.Context, projectID int64, boardID int64, req dto.CreateLabelRequest) (*model.Label, error)
	DeleteLabel(ctx context.Context, projectID int64, boardID int64, labelID int64) error
	ToggleLabel(ctx context.Context, projectID int64, boardID int64, cardID int64, labelID int64) (string, error)
}

type LabelService struct {
	repo         repository.LabelRepositoryInterface
	permSvc      *PermissionService
	activityRepo repository.ActivityRepositoryInterface
	boardRepo    repository.BoardRepositoryInterface
	cardRepo     repository.CardRepositoryInterface
	columnRepo   repository.ColumnRepositoryInterface
}

func NewLabelService(
	repo repository.LabelRepositoryInterface,
	permSvc *PermissionService,
	activityRepo repository.ActivityRepositoryInterface,
	boardRepo repository.BoardRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
) *LabelService {
	return &LabelService{
		repo:         repo,
		permSvc:      permSvc,
		activityRepo: activityRepo,
		boardRepo:    boardRepo,
		cardRepo:     cardRepo,
		columnRepo:   columnRepo,
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
	return s.repo.CreateLabel(ctx, boardID, l)
}

func (s *LabelService) DeleteLabel(ctx context.Context, projectID int64, boardID int64, labelID int64) error {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	if _, err := s.getLabelInBoard(ctx, boardID, labelID); err != nil {
		return err
	}
	return s.repo.DeleteLabel(ctx, labelID)
}

func (s *LabelService) ToggleLabel(ctx context.Context, projectID int64, boardID int64, cardID int64, labelID int64) (string, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return "", err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return "", err
	}
	if err := s.ensureCardInBoard(ctx, boardID, cardID); err != nil {
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

	if added {
		s.logActivity(ctx, cardID, "label_added", nil, &label.Name)
		return "attached", nil
	}
	s.logActivity(ctx, cardID, "label_removed", &label.Name, nil)
	return "detached", nil
}

func (s *LabelService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.boardRepo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, mapNoRowsToNotFound(err)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.ErrNotFound
	}
	return board, nil
}

func (s *LabelService) getLabelInBoard(ctx context.Context, boardID int64, labelID int64) (*model.Label, error) {
	label, err := s.repo.GetLabel(ctx, labelID)
	if err != nil {
		return nil, mapNoRowsToNotFound(err)
	}
	if label.BoardID != boardID {
		return nil, apperr.ErrNotFound
	}
	return label, nil
}

func (s *LabelService) ensureCardInBoard(ctx context.Context, boardID int64, cardID int64) error {
	card, err := s.cardRepo.GetCard(ctx, cardID)
	if err != nil {
		return mapNoRowsToNotFound(err)
	}
	column, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return mapNoRowsToNotFound(err)
	}
	if column.BoardID != boardID {
		return apperr.ErrNotFound
	}
	return nil
}

func normalizeLabelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", apperr.New(apperr.CodeValidation, "label name required")
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

func (s *LabelService) logActivity(ctx context.Context, cardID int64, action string, oldValue, newValue *string) {
	_ = s.activityRepo.LogActivity(ctx, cardID, currentUserID(ctx), action, oldValue, newValue)
}
