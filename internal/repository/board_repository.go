package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type BoardRepositoryInterface interface {
	GetBoardsByProject(ctx context.Context, projectID int64) ([]model.Board, error)
	CreateBoard(ctx context.Context, projectID int64, b *model.Board) (*model.Board, error)
	GetBoard(ctx context.Context, boardID int64) (*model.Board, error)
	UpdateBoard(ctx context.Context, b *model.Board) (*model.Board, error)
	DeleteBoard(ctx context.Context, boardID int64) error
	GetBoardArchive(ctx context.Context, boardID int64, filters model.BoardArchiveFilters) (*model.BoardArchivePage, error)
	HasColumnsByBoard(ctx context.Context, boardID int64) (bool, error)
	HasActiveCardsByBoard(ctx context.Context, boardID int64) (bool, error)
	NextBoardID(ctx context.Context, projectID int64, excludeBoardID int64) (*int64, error)
}

type BoardRepository struct {
	Db *pgxpool.Pool
}

func NewBoardRepository(db *pgxpool.Pool) *BoardRepository {
	return &BoardRepository{
		Db: db,
	}
}

func (r *BoardRepository) GetBoardsByProject(ctx context.Context, projectID int64) ([]model.Board, error) {
	queries := dbgen.New(r.Db)
	dbBoards, err := queries.GetBoardsByProject(ctx, int32(projectID))
	if err != nil {
		return nil, err
	}

	var boards []model.Board
	for _, b := range dbBoards {
		boards = append(boards, model.Board{
			ID:              int64(b.ID),
			Title:           b.Title,
			Position:        b.Position,
			KanbanProjectID: int64(b.KanbanProjectID),
			CreatedByID:     int64(b.CreatedByID),
			CreatedAt:       b.CreatedAt.Time,
			UpdatedAt:       b.UpdatedAt.Time,
		})
	}
	return boards, nil
}

func (r *BoardRepository) CreateBoard(ctx context.Context, projectID int64, b *model.Board) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateBoard(ctx, dbgen.CreateBoardParams{
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: int32(projectID),
		CreatedByID:     int32(b.CreatedByID),
	})
	if err != nil {
		return nil, err
	}

	b.ID = int64(res.ID)
	b.Title = res.Title
	b.Position = res.Position
	b.KanbanProjectID = int64(res.KanbanProjectID)
	b.CreatedByID = int64(res.CreatedByID)
	b.CreatedAt = res.CreatedAt.Time
	b.UpdatedAt = res.UpdatedAt.Time
	if res.DeletedAt.Valid {
		b.DeletedAt = &res.DeletedAt.Time
	}
	return b, nil
}

func (r *BoardRepository) GetBoard(ctx context.Context, id int64) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	b, err := queries.GetBoard(ctx, int32(id))
	if err != nil {
		return nil, err
	}

	return &model.Board{
		ID:              int64(b.ID),
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: int64(b.KanbanProjectID),
		CreatedByID:     int64(b.CreatedByID),
		CreatedAt:       b.CreatedAt.Time,
		UpdatedAt:       b.UpdatedAt.Time,
	}, nil
}

func (r *BoardRepository) UpdateBoard(ctx context.Context, b *model.Board) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateBoard(ctx, dbgen.UpdateBoardParams{
		Title:    b.Title,
		Position: b.Position,
		ID:       int32(b.ID),
	})
	if err != nil {
		return nil, err
	}

	b.ID = int64(res.ID)
	b.Title = res.Title
	b.Position = res.Position
	b.KanbanProjectID = int64(res.KanbanProjectID)
	b.CreatedByID = int64(res.CreatedByID)
	b.CreatedAt = res.CreatedAt.Time
	b.UpdatedAt = res.UpdatedAt.Time
	if res.DeletedAt.Valid {
		b.DeletedAt = &res.DeletedAt.Time
	} else {
		b.DeletedAt = nil
	}
	return b, nil
}

func (r *BoardRepository) DeleteBoard(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteBoard(ctx, int32(id))
}

func (r *BoardRepository) GetBoardArchive(ctx context.Context, boardID int64, filters model.BoardArchiveFilters) (*model.BoardArchivePage, error) {
	page := filters.Page
	if page < 1 {
		page = 1
	}
	limit := filters.Limit
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	baseQuery := buildArchivedCardsQuery(boardID, filters)

	totalSQL, totalArgs, err := baseQuery.Columns("COUNT(c.id)").ToSql()
	if err != nil {
		return nil, err
	}
	var total int64
	if err := r.Db.QueryRow(ctx, totalSQL, totalArgs...).Scan(&total); err != nil {
		return nil, err
	}

	archivedCountSQL, archivedCountArgs, err := sq.Select("COUNT(c.id)").
		PlaceholderFormat(sq.Dollar).
		From("kanban_card c").
		Join("kanban_column col ON c.column_id = col.id").
		Where(sq.Eq{"col.board_id": boardID}).
		Where(sq.Eq{"c.is_archived": true}).
		ToSql()
	if err != nil {
		return nil, err
	}
	var archivedCount int64
	if err := r.Db.QueryRow(ctx, archivedCountSQL, archivedCountArgs...).Scan(&archivedCount); err != nil {
		return nil, err
	}

	archiveSQL, archiveArgs, err := baseQuery.
		Columns(
			"c.id",
			"c.title",
			"c.description",
			"col.title AS column_title",
			"c.border_color",
			"c.archived_at",
			"c.archived_by_id",
			"u.lastname",
			"u.firstname",
			"u.avatar_name",
		).
		OrderBy("c.archived_at DESC NULLS LAST", "c.id DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := r.Db.Query(ctx, archiveSQL, archiveArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.ArchivedCard, 0, limit)
	for rows.Next() {
		var card model.ArchivedCard
		var description pgtype.Text
		var borderColor pgtype.Text
		var archivedAt pgtype.Timestamp
		var archivedByID pgtype.Int4
		var archivedByLastname pgtype.Text
		var archivedByFirstname pgtype.Text
		var archivedByAvatar pgtype.Text

		if err := rows.Scan(
			&card.ID,
			&card.Title,
			&description,
			&card.ColumnTitle,
			&borderColor,
			&archivedAt,
			&archivedByID,
			&archivedByLastname,
			&archivedByFirstname,
			&archivedByAvatar,
		); err != nil {
			return nil, err
		}

		if description.Valid {
			card.Description = &description.String
		}
		if borderColor.Valid {
			card.BorderColor = &borderColor.String
		}
		if archivedAt.Valid {
			card.ArchivedAt = &archivedAt.Time
		}
		if archivedByID.Valid {
			user := &model.User{ID: int64(archivedByID.Int32)}
			if archivedByLastname.Valid {
				user.Lastname = archivedByLastname.String
			}
			if archivedByFirstname.Valid {
				user.Firstname = archivedByFirstname.String
			}
			if archivedByAvatar.Valid {
				user.AvatarName = &archivedByAvatar.String
			}
			card.ArchivedBy = user
		}

		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.BoardArchivePage{
		Cards:         cards,
		Page:          page,
		Limit:         limit,
		Total:         total,
		ArchivedCount: archivedCount,
	}, nil
}

func buildArchivedCardsQuery(boardID int64, filters model.BoardArchiveFilters) sq.SelectBuilder {
	query := sq.Select().
		PlaceholderFormat(sq.Dollar).
		From("kanban_card c").
		Join("kanban_column col ON c.column_id = col.id").
		LeftJoin("users u ON c.archived_by_id = u.id").
		Where(sq.Eq{"col.board_id": boardID}).
		Where(sq.Eq{"c.is_archived": true})

	if title := strings.TrimSpace(filters.Title); title != "" {
		query = query.Where(sq.Like{"c.title": "%" + title + "%"})
	}
	if description := strings.TrimSpace(filters.Description); description != "" {
		query = query.Where(sq.Like{"c.description": "%" + description + "%"})
	}
	if from, ok := parseArchiveDate(filters.DateFrom, false); ok {
		query = query.Where(sq.GtOrEq{"c.archived_at": from})
	}
	if to, ok := parseArchiveDate(filters.DateTo, true); ok {
		query = query.Where(sq.LtOrEq{"c.archived_at": to})
	}

	return query
}

func parseArchiveDate(value string, endOfDay bool) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	if endOfDay {
		date = date.Add(24 * time.Hour).Add(-time.Second)
	}
	return date, true
}

func (r *BoardRepository) HasColumnsByBoard(ctx context.Context, boardID int64) (bool, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.HasColumnsByBoard(ctx, int32(boardID))
	if err != nil {
		return false, err
	}
	return res, nil
}

func (r *BoardRepository) HasActiveCardsByBoard(ctx context.Context, boardID int64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM kanban_card c
			JOIN kanban_column col ON c.column_id = col.id
			WHERE col.board_id = $1
				AND c.is_archived = FALSE
		)`
	var exists bool
	if err := r.Db.QueryRow(ctx, query, boardID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *BoardRepository) NextBoardID(ctx context.Context, projectID int64, excludeBoardID int64) (*int64, error) {
	query := `
		SELECT id
		FROM kanban_board
		WHERE kanban_project_id = $1
			AND id <> $2
			AND deleted_at IS NULL
		ORDER BY position ASC, id ASC
		LIMIT 1`
	var id int64
	if err := r.Db.QueryRow(ctx, query, projectID, excludeBoardID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &id, nil
}
