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

type ColumnServiceInterface interface {
	CreateColumn(ctx context.Context, projectID int64, boardID int64, req dto.CreateColumnRequest) (*model.Column, error)
	// UpdateColumn возвращает обновлённую колонку и, если смена позиции вызвала
	// ребалансировку доски, все её колонки с новыми позициями (иначе nil).
	UpdateColumn(ctx context.Context, projectID int64, boardID int64, columnID int64, req dto.UpdateColumnRequest) (*model.Column, []model.Column, error)
	DeleteColumn(ctx context.Context, projectID int64, boardID int64, columnID int64) error
}

type ColumnService struct {
	repo      repository.ColumnRepositoryInterface
	permSvc   *PermissionService
	boardRepo repository.BoardRepositoryInterface
	History   HistoryLogger
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
	created, err := s.repo.CreateColumn(ctx, boardID, c)
	if err == nil && created != nil {
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "column.created",
			EntityType:  "column",
			EntityID:    created.ID,
			EntityTitle: created.Title,
		})
	}
	return created, err
}

func (s *ColumnService) UpdateColumn(ctx context.Context, projectID int64, boardID int64, columnID int64, req dto.UpdateColumnRequest) (*model.Column, []model.Column, error) {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return nil, nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, nil, err
	}

	c, err := s.getColumnInBoard(ctx, boardID, columnID)
	if err != nil {
		return nil, nil, err
	}
	oldTitle, oldColor, oldPos := c.Title, c.HeaderColor, c.Position

	title := ""
	hasTitle := false
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
		hasTitle = title != ""
	}
	hasHeaderColor := req.HeaderColor != nil
	hasPosition := req.Position != nil
	if !hasTitle && !hasHeaderColor && !hasPosition {
		return nil, nil, apperr.New(apperr.CodeUpdateFieldsRequired, "update fields required")
	}

	titleChanged := hasTitle && title != oldTitle
	colorChanged := false
	if hasHeaderColor {
		if color, ok := normalizeColumnColorForUpdate(*req.HeaderColor); ok {
			colorChanged = color != oldColor
			c.HeaderColor = color
		}
	}
	if hasTitle {
		c.Title = title
	}
	if hasPosition {
		c.Position = *req.Position
	}
	updated, err := s.repo.UpdateColumn(ctx, c)
	if err != nil || updated == nil {
		return updated, nil, err
	}
	posChanged := hasPosition && updated.Position != oldPos

	var columns []model.Column
	if hasPosition {
		// Позиция менялась — проверяем, не слиплись ли соседи после вставки.
		rebalanced, rErr := s.repo.RebalanceBoardColumns(ctx, boardID)
		if rErr != nil {
			return nil, nil, rErr
		}
		if rebalanced {
			// Перенумерация переписала позиции всей доски: перечитываем и колонку,
			// и список, иначе у клиента останется позиция, которой в базе уже нет.
			if updated, err = s.repo.GetColumn(ctx, columnID); err != nil {
				return nil, nil, err
			}
			if columns, err = s.repo.GetColumnsByBoard(ctx, boardID); err != nil {
				return nil, nil, err
			}
			posChanged = true
		}
	}

	if !titleChanged && !colorChanged && !posChanged {
		return updated, columns, nil
	}
	action := "column.updated"
	before, after := "", ""
	switch {
	case titleChanged && !colorChanged && !posChanged:
		action = "column.updated.renamed"
		before, after = oldTitle, title
	case colorChanged && !titleChanged && !posChanged:
		action = "column.updated.color"
		before, after = oldColor, c.HeaderColor
	case posChanged && !titleChanged && !colorChanged:
		action = "column.updated.moved"
		before, after = historyPos(oldPos), historyPos(updated.Position)
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   projectID,
		Action:      action,
		EntityType:  "column",
		EntityID:    columnID,
		EntityTitle: updated.Title,
		Before:      before,
		After:       after,
	})
	return updated, columns, nil
}

func (s *ColumnService) DeleteColumn(ctx context.Context, projectID int64, boardID int64, columnID int64) error {
	if _, err := s.resolveBoard(ctx, projectID, boardID); err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}
	col, err := s.getColumnInBoard(ctx, boardID, columnID)
	if err != nil {
		return err
	}

	hasCards, err := s.repo.HasCardsByColumn(ctx, columnID)
	if err != nil {
		return err
	}
	if hasCards {
		return apperr.New(apperr.CodeColumnHasCards, "cannot delete column with active cards")
	}

	err = s.repo.DeleteColumn(ctx, columnID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeColumnHasCards, "cannot delete column with cards")
		}
		return err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   projectID,
		Action:      "column.deleted",
		EntityType:  "column",
		EntityID:    columnID,
		EntityTitle: col.Title,
		EntityLink:  historyBoardPath(projectID, boardID),
	})
	return nil
}

func (s *ColumnService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.boardRepo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeBoardNotFound)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.New(apperr.CodeBoardNotFound, "board not found")
	}
	return board, nil
}

func (s *ColumnService) getColumnInBoard(ctx context.Context, boardID int64, columnID int64) (*model.Column, error) {
	column, err := s.repo.GetColumn(ctx, columnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}
	if column.BoardID != boardID {
		return nil, apperr.New(apperr.CodeColumnNotFound, "column not found")
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
		return "", apperr.New(apperr.CodeColumnTitleRequired, "column title required")
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
