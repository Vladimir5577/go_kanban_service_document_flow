package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type ColumnRepositoryInterface interface {
	CreateColumn(ctx context.Context, boardID int64, c *model.Column) (*model.Column, error)
	UpdateColumn(ctx context.Context, c *model.Column) (*model.Column, error)
	DeleteColumn(ctx context.Context, id int64) error
	GetColumn(ctx context.Context, id int64) (*model.Column, error)
	GetColumnsByBoard(ctx context.Context, boardID int64) ([]model.Column, error)
	HasCardsByColumn(ctx context.Context, columnID int64) (bool, error)

	// RebalanceBoardColumns перенумеровывает колонки доски, если позиции соседей
	// сблизились настолько, что вставить между ними уже нечего. Возвращает true,
	// если позиции были переписаны.
	RebalanceBoardColumns(ctx context.Context, boardID int64) (bool, error)
}

type ColumnRepository struct {
	Db *pgxpool.Pool
}

func NewColumnRepository(db *pgxpool.Pool) *ColumnRepository {
	return &ColumnRepository{
		Db: db,
	}
}

func (r *ColumnRepository) GetColumnsByBoard(ctx context.Context, boardID int64) ([]model.Column, error) {
	queries := dbgen.New(r.Db)
	dbCols, err := queries.GetColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	var cols []model.Column
	for _, c := range dbCols {
		cols = append(cols, model.Column{
			ID:          c.ID,
			Title:       c.Title,
			HeaderColor: c.HeaderColor,
			Position:    c.Position,
			BoardID:     c.BoardID,
		})
	}
	return cols, nil
}

func (r *ColumnRepository) CreateColumn(ctx context.Context, boardID int64, c *model.Column) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateColumn(ctx, dbgen.CreateColumnParams{
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     boardID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.Title = res.Title
	c.HeaderColor = res.HeaderColor
	c.Position = res.Position
	c.BoardID = res.BoardID
	return c, nil
}

func (r *ColumnRepository) GetColumn(ctx context.Context, id int64) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetColumn(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	return &model.Column{
		ID:          c.ID,
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     c.BoardID,
	}, nil
}

func (r *ColumnRepository) UpdateColumn(ctx context.Context, c *model.Column) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateColumn(ctx, dbgen.UpdateColumnParams{
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		ID:          c.ID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.Title = res.Title
	c.HeaderColor = res.HeaderColor
	c.Position = res.Position
	c.BoardID = res.BoardID
	return c, nil
}

func (r *ColumnRepository) DeleteColumn(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteColumn(ctx, id)
}

func (r *ColumnRepository) RebalanceBoardColumns(ctx context.Context, boardID int64) (bool, error) {
	rows, err := r.Db.Query(ctx, `
		SELECT position
		FROM kanban_column
		WHERE board_id = $1
		ORDER BY position ASC, id ASC`, boardID)
	if err != nil {
		return false, NormalizeError(err)
	}
	defer rows.Close()

	positions := make([]float64, 0)
	for rows.Next() {
		var p float64
		if err := rows.Scan(&p); err != nil {
			return false, err
		}
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	const epsilon = 0.0001
	needsRebalance := false
	for i := 1; i < len(positions); i++ {
		if positions[i]-positions[i-1] < epsilon {
			needsRebalance = true
			break
		}
	}
	if !needsRebalance {
		return false, nil
	}

	// Шаг 65536, как у RebalanceColumnCards: запас на вставки между соседями,
	// чтобы следующая ребалансировка потребовалась не скоро.
	//
	// ponytail: отдельная транзакция не нужна — UPDATE сам пересчитывает ранги,
	// а устаревший результат проверки выше в худшем случае даст лишнюю
	// перенумерацию либо пропуск, который добьёт следующее перемещение.
	if _, err := r.Db.Exec(ctx, `
		WITH ranked AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY position ASC, id ASC) AS rn
			FROM kanban_column
			WHERE board_id = $1
		)
		UPDATE kanban_column
		SET position = ranked.rn * 65536.0
		FROM ranked
		WHERE kanban_column.id = ranked.id`, boardID); err != nil {
		return false, NormalizeError(err)
	}
	return true, nil
}

func (r *ColumnRepository) HasCardsByColumn(ctx context.Context, columnID int64) (bool, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.HasCardsByColumn(ctx, columnID)
	if err != nil {
		return false, err
	}
	return res, nil
}
