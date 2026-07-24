package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CardRepositoryInterface interface {
	CreateCard(ctx context.Context, columnID int64, c *model.Card) (*model.Card, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error)
	GetCardsByBoard(ctx context.Context, boardID int64) ([]model.Card, error)
	GetAssignedCards(ctx context.Context, userID int64, status string) ([]AssignedCardRow, error)
	GetAssignedSubtasks(ctx context.Context, userID int64, status string) ([]AssignedSubtaskRow, error)
	CountActiveCardsByBoard(ctx context.Context, boardID int64) (int, error)
	GetAssigneesByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	GetLabelIDsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error
	MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error)

	// GetInvolvedUserIDsForNotifications returns distinct user IDs that are assignees on the card,
	// assignees on any of its subtasks, or the card's author. Used to decide notification recipients.
	GetInvolvedUserIDsForNotifications(ctx context.Context, cardID int64) ([]int64, error)
}

type CardRepository struct {
	Db *pgxpool.Pool
}

type AssignedCardRow struct {
	ProjectID   int64
	ProjectName string
	BoardID     int64
	BoardTitle  string
	ColumnID    int64
	ColumnTitle string
	CardID      int64
	CardTitle   string
	Priority    *string
	DueDate     *time.Time
	BorderColor *string
}

type AssignedSubtaskRow struct {
	SubtaskID     int64
	SubtaskTitle  string
	SubtaskStatus string
	CardID        int64
	CardTitle     string
	ColumnID      int64
	ColumnTitle   string
	BoardID       int64
	BoardTitle    string
	ProjectID     int64
	ProjectName   string
}

func NewCardRepository(db *pgxpool.Pool) *CardRepository {
	return &CardRepository{
		Db: db,
	}
}

func (r *CardRepository) GetAssignedCards(ctx context.Context, userID int64, status string) ([]AssignedCardRow, error) {
	queries := dbgen.New(r.Db)

	switch status {
	case "open":
		rows, err := queries.GetAssignedCardsOpen(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]AssignedCardRow, 0, len(rows))
		for _, row := range rows {
			result = append(result, assignedCardRowFromOpen(row))
		}
		return result, nil
	case "closed":
		rows, err := queries.GetAssignedCardsClosed(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]AssignedCardRow, 0, len(rows))
		for _, row := range rows {
			result = append(result, assignedCardRowFromClosed(row))
		}
		return result, nil
	default:
		return nil, apperr.New(apperr.CodeValidation, "invalid status filter")
	}
}

func (r *CardRepository) GetAssignedSubtasks(ctx context.Context, userID int64, status string) ([]AssignedSubtaskRow, error) {
	queries := dbgen.New(r.Db)

	switch status {
	case "open":
		rows, err := queries.GetAssignedSubtasksOpen(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]AssignedSubtaskRow, 0, len(rows))
		for _, row := range rows {
			result = append(result, assignedSubtaskRowFromOpen(row))
		}
		return result, nil
	case "closed":
		rows, err := queries.GetAssignedSubtasksClosed(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]AssignedSubtaskRow, 0, len(rows))
		for _, row := range rows {
			result = append(result, assignedSubtaskRowFromClosed(row))
		}
		return result, nil
	default:
		return nil, apperr.New(apperr.CodeValidation, "invalid status filter")
	}
}

func assignedCardRowFromOpen(row dbgen.GetAssignedCardsOpenRow) AssignedCardRow {
	return AssignedCardRow{
		ProjectID:   row.ProjectID,
		ProjectName: row.ProjectName,
		BoardID:     row.BoardID,
		BoardTitle:  row.BoardTitle,
		ColumnID:    row.ColumnID,
		ColumnTitle: row.ColumnTitle,
		CardID:      row.CardID,
		CardTitle:   row.CardTitle,
		Priority:    textPtr(row.CardPriority),
		DueDate:     timestamptzPtr(row.CardDueDate),
		BorderColor: textPtr(row.CardBorderColor),
	}
}

func assignedCardRowFromClosed(row dbgen.GetAssignedCardsClosedRow) AssignedCardRow {
	return AssignedCardRow{
		ProjectID:   row.ProjectID,
		ProjectName: row.ProjectName,
		BoardID:     row.BoardID,
		BoardTitle:  row.BoardTitle,
		ColumnID:    row.ColumnID,
		ColumnTitle: row.ColumnTitle,
		CardID:      row.CardID,
		CardTitle:   row.CardTitle,
		Priority:    textPtr(row.CardPriority),
		DueDate:     timestamptzPtr(row.CardDueDate),
		BorderColor: textPtr(row.CardBorderColor),
	}
}

func assignedSubtaskRowFromOpen(row dbgen.GetAssignedSubtasksOpenRow) AssignedSubtaskRow {
	return AssignedSubtaskRow{
		SubtaskID:     row.SubtaskID,
		SubtaskTitle:  row.SubtaskTitle,
		SubtaskStatus: row.SubtaskStatus,
		CardID:        row.CardID,
		CardTitle:     row.CardTitle,
		ColumnID:      row.ColumnID,
		ColumnTitle:   row.ColumnTitle,
		BoardID:       row.BoardID,
		BoardTitle:    row.BoardTitle,
		ProjectID:     row.ProjectID,
		ProjectName:   row.ProjectName,
	}
}

func assignedSubtaskRowFromClosed(row dbgen.GetAssignedSubtasksClosedRow) AssignedSubtaskRow {
	return AssignedSubtaskRow{
		SubtaskID:     row.SubtaskID,
		SubtaskTitle:  row.SubtaskTitle,
		SubtaskStatus: row.SubtaskStatus,
		CardID:        row.CardID,
		CardTitle:     row.CardTitle,
		ColumnID:      row.ColumnID,
		ColumnTitle:   row.ColumnTitle,
		BoardID:       row.BoardID,
		BoardTitle:    row.BoardTitle,
		ProjectID:     row.ProjectID,
		ProjectName:   row.ProjectName,
	}
}

func textPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	value := v.String
	return &value
}

func timestamptzPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	value := v.Time
	return &value
}

func (r *CardRepository) CountActiveCardsByBoard(ctx context.Context, boardID int64) (int, error) {
	query := `
		SELECT COUNT(c.id)
		FROM kanban_card c
		JOIN kanban_column col ON col.id = c.column_id
		WHERE col.board_id = $1 AND c.is_archived = FALSE`

	var count int
	if err := r.Db.QueryRow(ctx, query, boardID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *CardRepository) GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetCardsByColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}

	if len(dbCards) == 0 {
		return []model.Card{}, nil
	}

	// Собрать card IDs для bulk-запросов
	cardIDs := make([]int64, len(dbCards))
	for i, c := range dbCards {
		cardIDs[i] = c.ID
	}

	// Bulk-запросы assignees и labels для всех карточек
	assigneesByCard, err := r.GetAssigneesByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	labelsByCard, err := r.GetLabelIDsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	var cards []model.Card
	for _, c := range dbCards {
		card := model.Card{
			ID:         c.ID,
			Title:      c.Title,
			Position:   c.Position,
			IsArchived: c.IsArchived,
			ColumnID:   c.ColumnID,
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
			t := c.DueDate.Time
			card.DueDate = &t
		}
		if c.ArchivedAt.Valid {
			t := c.ArchivedAt.Time
			card.ArchivedAt = &t
		}
		if c.ArchivedByID.Valid {
			v := c.ArchivedByID.Int64
			card.ArchivedByID = &v
		}
		if c.CompletedAt.Valid {
			t := c.CompletedAt.Time
			card.CompletedAt = &t
		}
		if c.CompletedByID.Valid {
			v := c.CompletedByID.Int64
			card.CompletedByID = &v
		}
		if c.CreatedByID.Valid {
			v := c.CreatedByID.Int64
			card.CreatedByID = &v
		}

		// Проставить assignees и labels из bulk-результатов
		card.AssigneeIDs = assigneesByCard[card.ID]
		card.LabelIDs = labelsByCard[card.ID]

		cards = append(cards, card)
	}
	return cards, nil
}

func (r *CardRepository) GetCardsByBoard(ctx context.Context, boardID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetCardsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	var cards []model.Card
	for _, c := range dbCards {
		card := model.Card{
			ID:         c.ID,
			Title:      c.Title,
			Position:   c.Position,
			IsArchived: c.IsArchived,
			ColumnID:   c.ColumnID,
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
			t := c.DueDate.Time
			card.DueDate = &t
		}
		if c.ArchivedAt.Valid {
			t := c.ArchivedAt.Time
			card.ArchivedAt = &t
		}
		if c.ArchivedByID.Valid {
			v := c.ArchivedByID.Int64
			card.ArchivedByID = &v
		}
		if c.CompletedAt.Valid {
			t := c.CompletedAt.Time
			card.CompletedAt = &t
		}
		if c.CompletedByID.Valid {
			v := c.CompletedByID.Int64
			card.CompletedByID = &v
		}
		if c.CreatedByID.Valid {
			v := c.CreatedByID.Int64
			card.CreatedByID = &v
		}

		cards = append(cards, card)
	}
	return cards, nil
}

func (r *CardRepository) GetAssigneesByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error) {
	if len(cardIDs) == 0 {
		return make(map[int64][]int64), nil
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetCardAssigneesByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64][]int64)
	for _, row := range rows {
		result[row.CardID] = append(result[row.CardID], row.UserID)
	}

	return result, nil
}

func (r *CardRepository) GetLabelIDsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error) {
	if len(cardIDs) == 0 {
		return make(map[int64][]int64), nil
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetCardLabelsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64][]int64)
	for _, row := range rows {
		result[row.KanbanCardID] = append(result[row.KanbanCardID], row.KanbanLabelID)
	}

	return result, nil
}

func (r *CardRepository) CreateCard(ctx context.Context, columnID int64, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateCardParams{
		Title:    c.Title,
		Position: c.Position,
		ColumnID: columnID,
	}
	if c.Description != nil {
		params.Description = pgtype.Text{String: *c.Description, Valid: true}
	}
	if c.DueDate != nil {
		params.DueDate = pgtype.Timestamptz{Time: *c.DueDate, Valid: true}
	}
	if c.Priority != nil {
		params.Priority = pgtype.Text{String: *c.Priority, Valid: true}
	}
	if c.CreatedByID != nil {
		params.CreatedByID = pgtype.Int8{Int64: *c.CreatedByID, Valid: true}
	}
	if c.BorderColor != nil {
		params.BorderColor = pgtype.Text{String: *c.BorderColor, Valid: true}
	}

	res, err := queries.CreateCard(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.CreatedAt = res.CreatedAt.Time
	c.UpdatedAt = res.UpdatedAt.Time
	return c, nil
}

func (r *CardRepository) GetCard(ctx context.Context, id int64) (*model.Card, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetCard(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	card := &model.Card{
		ID:         c.ID,
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		ColumnID:   c.ColumnID,
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
		t := c.DueDate.Time
		card.DueDate = &t
	}
	if c.ArchivedAt.Valid {
		t := c.ArchivedAt.Time
		card.ArchivedAt = &t
	}
	if c.ArchivedByID.Valid {
		v := c.ArchivedByID.Int64
		card.ArchivedByID = &v
	}
	if c.CompletedAt.Valid {
		t := c.CompletedAt.Time
		card.CompletedAt = &t
	}
	if c.CompletedByID.Valid {
		v := c.CompletedByID.Int64
		card.CompletedByID = &v
	}
	if c.CreatedByID.Valid {
		v := c.CreatedByID.Int64
		card.CreatedByID = &v
	}

	// Bulk-запросы для assignees и labels (используем существующие методы)
	assigneesByCard, err := r.GetAssigneesByCardIDs(ctx, []int64{card.ID})
	if err != nil {
		return nil, err
	}
	card.AssigneeIDs = assigneesByCard[card.ID]

	labelsByCard, err := r.GetLabelIDsByCardIDs(ctx, []int64{card.ID})
	if err != nil {
		return nil, err
	}
	card.LabelIDs = labelsByCard[card.ID]

	return card, nil
}

func (r *CardRepository) UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.UpdateCardParams{
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		ColumnID:   c.ColumnID,
		ID:         c.ID,
	}
	if c.Description != nil {
		params.Description = pgtype.Text{String: *c.Description, Valid: true}
	}
	if c.DueDate != nil {
		params.DueDate = pgtype.Timestamptz{Time: *c.DueDate, Valid: true}
	}
	if c.Priority != nil {
		params.Priority = pgtype.Text{String: *c.Priority, Valid: true}
	}
	if c.BorderColor != nil {
		params.BorderColor = pgtype.Text{String: *c.BorderColor, Valid: true}
	}
	if c.ArchivedAt != nil {
		params.ArchivedAt = pgtype.Timestamptz{Time: *c.ArchivedAt, Valid: true}
	}
	if c.ArchivedByID != nil {
		params.ArchivedByID = pgtype.Int8{Int64: *c.ArchivedByID, Valid: true}
	}
	if c.CompletedAt != nil {
		params.CompletedAt = pgtype.Timestamptz{Time: *c.CompletedAt, Valid: true}
	}
	if c.CompletedByID != nil {
		params.CompletedByID = pgtype.Int8{Int64: *c.CompletedByID, Valid: true}
	}

	res, err := queries.UpdateCard(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.UpdatedAt = res.UpdatedAt.Time
	return c, nil
}

func (r *CardRepository) DeleteCard(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteCard(ctx, id)
}

func (r *CardRepository) UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error {
	queries := dbgen.New(r.Db)

	if err := queries.ClearCardAssignees(ctx, cardID); err != nil {
		return err
	}

	for _, uid := range userIDs {
		if err := queries.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{
			CardID: cardID,
			UserID: uid,
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
		if err := queries.RebalanceColumnCards(ctx, columnID); err != nil {
			return nil, err
		}
		// fetch card again to get the rebalanced position
		return r.GetCard(ctx, id)
	}

	return updatedCard, nil
}

// GetInvolvedUserIDsForNotifications returns distinct assignees + subtask users + card author.
func (r *CardRepository) GetInvolvedUserIDsForNotifications(ctx context.Context, cardID int64) ([]int64, error) {
	// Get direct assignees
	assigneeMap, err := r.GetAssigneesByCardIDs(ctx, []int64{cardID})
	if err != nil {
		return nil, err
	}
	ids := map[int64]bool{}
	for _, uid := range assigneeMap[cardID] {
		ids[uid] = true
	}

	// Get subtask users + card author in one round-trip (raw to avoid missing sqlc query).
	// Dedup is handled by the ids map below, so UNION ALL is enough.
	rows, err := r.Db.Query(ctx, `
		SELECT user_id FROM kanban_card_subtask WHERE card_id = $1 AND user_id IS NOT NULL
		UNION ALL
		SELECT created_by_id FROM kanban_card WHERE id = $1 AND created_by_id IS NOT NULL`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err == nil {
			ids[uid] = true
		}
	}

	result := make([]int64, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	return result, nil
}
