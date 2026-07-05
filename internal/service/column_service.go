package service

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const defaultColumnColor = "bg-primary"

var allowedColumnColors = map[string]struct{}{
	"bg-primary": {},
	"bg-warning": {},
	"bg-success": {},
	"bg-danger":  {},
	"bg-info":    {},
	"bg-dark":    {},
}

type ColumnServiceInterface interface {
	CreateColumn(ctx context.Context, projectID int64, boardID int64, req dto.CreateColumnRequest) (*model.Column, error)
	UpdateColumn(ctx context.Context, projectID int64, boardID int64, columnID int64, req dto.UpdateColumnRequest) (*model.Column, error)
	DeleteColumn(ctx context.Context, projectID int64, boardID int64, columnID int64) error
}

type ColumnService struct {
	repo      repository.ColumnRepositoryInterface
	permSvc   *PermissionService
	boardRepo repository.BoardRepositoryInterface
}

func NewColumnService(repo repository.ColumnRepositoryInterface, permSvc *PermissionService, boardRepo repository.BoardRepositoryInterface) *ColumnService {
	return &ColumnService{
		repo:      repo,
		permSvc:   permSvc,
		boardRepo: boardRepo,
	}
}

func (s *ColumnService) CreateColumn(ctx context.Context, projectID int64, boardID int64, req dto.CreateColumnRequest) (*model.Column, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return nil, err
	}

	title, err := normalizeColumnTitle(req.Title)
	if err != nil {
		return nil, err
	}

	position, err := s.nextColumnPosition(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if req.Position != nil {
		position = *req.Position
	}

	c := &model.Column{
		Title:       title,
		HeaderColor: normalizeColumnColorForCreate(req.HeaderColor),
		Position:    position,
		BoardID:     boardID,
	}
	return s.repo.CreateColumn(ctx, boardID, c)
}

func (s *ColumnService) UpdateColumn(ctx context.Context, projectID int64, boardID int64, columnID int64, req dto.UpdateColumnRequest) (*model.Column, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	c, err := s.getColumnInBoard(ctx, boardID, columnID)
	if err != nil {
		return nil, err
	}

	title := ""
	hasTitle := false
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
		hasTitle = title != ""
	}
	hasHeaderColor := req.HeaderColor != nil
	hasPosition := req.Position != nil
	if !hasTitle && !hasHeaderColor && !hasPosition {
		return nil, apperr.New(apperr.CodeValidation, "update fields required")
	}

	if hasTitle {
		c.Title = title
	}
	if hasHeaderColor {
		if color, ok := normalizeColumnColorForUpdate(*req.HeaderColor); ok {
			c.HeaderColor = color
		}
	}
	if hasPosition {
		c.Position = *req.Position
	}
	return s.repo.UpdateColumn(ctx, c)
}

func (s *ColumnService) DeleteColumn(ctx context.Context, projectID int64, boardID int64, columnID int64) error {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}
	if _, err := s.getColumnInBoard(ctx, boardID, columnID); err != nil {
		return err
	}

	hasCards, err := s.repo.HasCardsByColumn(ctx, columnID)
	if err != nil {
		return err
	}
	if hasCards {
		return apperr.New(apperr.CodeConflict, "cannot delete column with active cards")
	}

	err = s.repo.DeleteColumn(ctx, columnID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeValidation, "Нельзя удалить колонку, пока в ней есть задачи (включая архивные).")
		}
		return err
	}
	return nil
}

func (s *ColumnService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.boardRepo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, mapNoRowsToNotFound(err)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.ErrNotFound
	}
	return board, nil
}

func (s *ColumnService) getColumnInBoard(ctx context.Context, boardID int64, columnID int64) (*model.Column, error) {
	column, err := s.repo.GetColumn(ctx, columnID)
	if err != nil {
		return nil, mapNoRowsToNotFound(err)
	}
	if column.BoardID != boardID {
		return nil, apperr.ErrNotFound
	}
	return column, nil
}

func (s *ColumnService) nextColumnPosition(ctx context.Context, boardID int64) (float64, error) {
	columns, err := s.repo.GetColumnsByBoard(ctx, boardID)
	if err != nil {
		return 0, err
	}
	maxPosition := 0.0
	for _, column := range columns {
		maxPosition = math.Max(maxPosition, column.Position)
	}
	return maxPosition + 1.0, nil
}

func normalizeColumnTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", apperr.New(apperr.CodeValidation, "column title required")
	}
	return title, nil
}

func normalizeColumnColorForCreate(color *string) string {
	if color == nil {
		return defaultColumnColor
	}
	if normalized, ok := normalizeColumnColorForUpdate(*color); ok {
		return normalized
	}
	return defaultColumnColor
}

func normalizeColumnColorForUpdate(color string) (string, bool) {
	color = strings.TrimSpace(color)
	if _, ok := allowedColumnColors[color]; ok {
		return color, true
	}
	return "", false
}
