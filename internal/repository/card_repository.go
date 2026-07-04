package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CardRepositoryInterface interface {
	CreateCard(ctx context.Context, columnID int64, c *model.Card) (*model.Card, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error)
	UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error
	MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error)
	ArchiveCard(ctx context.Context, id int64) error
}

type CardRepository struct {
	Db *pgxpool.Pool
}

func NewCardRepository(db *pgxpool.Pool) *CardRepository {
	return &CardRepository{
		Db: db,
	}
}

func (r *CardRepository) GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetCardsByColumn(ctx, int32(columnID))
	if err != nil {
		return nil, err
	}

	var cards []model.Card
	for _, c := range dbCards {
		card := model.Card{
			ID:         int64(c.ID),
			Title:      c.Title,
			Position:   c.Position,
			IsArchived: c.IsArchived,
			ColumnID:   int64(c.ColumnID),
			CreatedAt:  c.CreatedAt.Time,
			UpdatedAt:  c.UpdatedAt.Time,
		}
		if c.Description.Valid {
			card.Description = &c.Description.String
		}
		if c.Priority.Valid {
			card.Priority = &c.Priority.String
		}
		if c.BorderColor.Valid {
			card.BorderColor = &c.BorderColor.String
		}
		if c.DueDate.Valid {
			card.DueDate = &c.DueDate.Time
		}
		if c.ArchivedAt.Valid {
			card.ArchivedAt = &c.ArchivedAt.Time
		}
		if c.ArchivedByID.Valid {
			v := int64(c.ArchivedByID.Int32)
			card.ArchivedByID = &v
		}
		if c.CompletedAt.Valid {
			card.CompletedAt = &c.CompletedAt.Time
		}
		if c.CompletedByID.Valid {
			v := int64(c.CompletedByID.Int32)
			card.CompletedByID = &v
		}
		if c.CreatedByID.Valid {
			v := int64(c.CreatedByID.Int32)
			card.CreatedByID = &v
		}

		// Получаем assignees
		assignees, _ := queries.GetCardAssignees(ctx, c.ID)
		var aIDs []int64
		for _, a := range assignees {
			aIDs = append(aIDs, int64(a))
		}
		card.AssigneeIDs = aIDs

		// Получаем метки
		labels, _ := queries.GetCardLabels(ctx, c.ID)
		var lIDs []int64
		for _, l := range labels {
			lIDs = append(lIDs, int64(l))
		}
		card.LabelIDs = lIDs

		cards = append(cards, card)
	}
	return cards, nil
}

func (r *CardRepository) CreateCard(ctx context.Context, columnID int64, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateCardParams{
		Title:    c.Title,
		Position: c.Position,
		ColumnID: int32(columnID),
	}
	if c.Description != nil {
		params.Description = pgtype.Text{String: *c.Description, Valid: true}
	}
	if c.DueDate != nil {
		params.DueDate = pgtype.Timestamp{Time: *c.DueDate, Valid: true}
	}
	if c.Priority != nil {
		params.Priority = pgtype.Text{String: *c.Priority, Valid: true}
	}
	if c.CreatedByID != nil {
		params.CreatedByID = pgtype.Int4{Int32: int32(*c.CreatedByID), Valid: true}
	}
	if c.BorderColor != nil {
		params.BorderColor = pgtype.Text{String: *c.BorderColor, Valid: true}
	}

	res, err := queries.CreateCard(ctx, params)
	if err != nil {
		return nil, err
	}

	c.ID = int64(res.ID)
	c.CreatedAt = res.CreatedAt.Time
	c.UpdatedAt = res.UpdatedAt.Time
	return c, nil
}

func (r *CardRepository) GetCard(ctx context.Context, id int64) (*model.Card, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetCard(ctx, int32(id))
	if err != nil {
		return nil, err
	}

	card := &model.Card{
		ID:         int64(c.ID),
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		ColumnID:   int64(c.ColumnID),
		CreatedAt:  c.CreatedAt.Time,
		UpdatedAt:  c.UpdatedAt.Time,
	}
	if c.Description.Valid {
		card.Description = &c.Description.String
	}
	if c.Priority.Valid {
		card.Priority = &c.Priority.String
	}
	if c.BorderColor.Valid {
		card.BorderColor = &c.BorderColor.String
	}
	if c.DueDate.Valid {
		card.DueDate = &c.DueDate.Time
	}
	if c.ArchivedAt.Valid {
		card.ArchivedAt = &c.ArchivedAt.Time
	}
	if c.ArchivedByID.Valid {
		v := int64(c.ArchivedByID.Int32)
		card.ArchivedByID = &v
	}
	if c.CompletedAt.Valid {
		card.CompletedAt = &c.CompletedAt.Time
	}
	if c.CompletedByID.Valid {
		v := int64(c.CompletedByID.Int32)
		card.CompletedByID = &v
	}
	if c.CreatedByID.Valid {
		v := int64(c.CreatedByID.Int32)
		card.CreatedByID = &v
	}

	assignees, _ := queries.GetCardAssignees(ctx, c.ID)
	var aIDs []int64
	for _, a := range assignees {
		aIDs = append(aIDs, int64(a))
	}
	card.AssigneeIDs = aIDs

	labels, _ := queries.GetCardLabels(ctx, c.ID)
	var lIDs []int64
	for _, l := range labels {
		lIDs = append(lIDs, int64(l))
	}
	card.LabelIDs = lIDs

	return card, nil
}

func (r *CardRepository) UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.UpdateCardParams{
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		ColumnID:   int32(c.ColumnID),
		ID:         int32(c.ID),
	}
	if c.Description != nil {
		params.Description = pgtype.Text{String: *c.Description, Valid: true}
	}
	if c.DueDate != nil {
		params.DueDate = pgtype.Timestamp{Time: *c.DueDate, Valid: true}
	}
	if c.Priority != nil {
		params.Priority = pgtype.Text{String: *c.Priority, Valid: true}
	}
	if c.BorderColor != nil {
		params.BorderColor = pgtype.Text{String: *c.BorderColor, Valid: true}
	}
	if c.ArchivedAt != nil {
		params.ArchivedAt = pgtype.Timestamp{Time: *c.ArchivedAt, Valid: true}
	}
	if c.ArchivedByID != nil {
		params.ArchivedByID = pgtype.Int4{Int32: int32(*c.ArchivedByID), Valid: true}
	}
	if c.CompletedAt != nil {
		params.CompletedAt = pgtype.Timestamp{Time: *c.CompletedAt, Valid: true}
	}
	if c.CompletedByID != nil {
		params.CompletedByID = pgtype.Int4{Int32: int32(*c.CompletedByID), Valid: true}
	}

	res, err := queries.UpdateCard(ctx, params)
	if err != nil {
		return nil, err
	}

	c.UpdatedAt = res.UpdatedAt.Time
	return c, nil
}

func (r *CardRepository) DeleteCard(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteCard(ctx, int32(id))
}

func (r *CardRepository) UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error {
	queries := dbgen.New(r.Db)

	if err := queries.ClearCardAssignees(ctx, int32(cardID)); err != nil {
		return err
	}

	for _, uid := range userIDs {
		if err := queries.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{
			CardID: int32(cardID),
			UserID: int32(uid),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *CardRepository) MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error) {
	// 1. Fetch card to check existence
	card, err := r.GetCard(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2. Fetch all cards in the destination column
	cards, err := r.GetCardsByColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}

	// 3. Check for collision
	const epsilon = 0.0001
	needsRebalance := false
	for _, c := range cards {
		if c.ID != id && (c.Position-position > -epsilon && c.Position-position < epsilon) {
			needsRebalance = true
			break
		}
	}

	// 4. Update card
	card.ColumnID = columnID
	card.Position = position

	updatedCard, err := r.UpdateCard(ctx, card)
	if err != nil {
		return nil, err
	}

	// 8. Trigger rebalance if needed
	if needsRebalance {
		queries := dbgen.New(r.Db)
		if err := queries.RebalanceColumnCards(ctx, int32(columnID)); err != nil {
			return nil, err
		}
		// fetch card again to get the rebalanced position
		return r.GetCard(ctx, id)
	}

	return updatedCard, nil
}

func (r *CardRepository) ArchiveCard(ctx context.Context, id int64) error {
	return nil
}
