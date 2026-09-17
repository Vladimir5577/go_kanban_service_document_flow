package repository

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CardRepositoryInterface interface {
	CreateCard(ctx context.Context, columnID int64, c *model.Card) (*model.Card, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error)
	GetCardsByBoard(ctx context.Context, boardID int64) ([]model.Card, error)
	ListTasks(ctx context.Context, f TaskListParams) ([]AssignedCardRow, int64, error)
	ListTaskCollaborants(ctx context.Context, f TaskCollaborantsParams) ([]model.User, int64, error)
	GetChildCards(ctx context.Context, parentID int64) ([]model.Card, error)
	GetChildStats(ctx context.Context, parentID int64) (total int64, maxPosition float64, err error)
	MoveChildCard(ctx context.Context, id int64, position float64) (*model.CardMove, error)
	GetChildCountsByParentIDs(ctx context.Context, parentIDs []int64) (map[int64]model.ChecklistCount, error)
	CountActiveCardsByBoard(ctx context.Context, boardID int64) (int, error)
	GetAssigneesByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	GetLabelIDsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error
	// MoveCard читает и пишет только поля, которые участвуют в перемещении.
	// Если оно вызвало ребалансировку, в CardMove.Rebalanced лягут новые позиции
	// всей колонки, иначе там nil.
	MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.CardMove, error)

	// GetInvolvedUserIDsForNotifications returns distinct user IDs that are assignees on the card,
	// assignees on any of its subtasks, or the card's author. Used to decide notification recipients.
	GetInvolvedUserIDsForNotifications(ctx context.Context, cardID int64) ([]int64, error)
}

type CardRepository struct {
	Db *pgxpool.Pool
}

type TaskListParams struct {
	ViewerID      int64
	TitleQuery    string
	ProjectID     int64
	Priority      string
	AuthorID      int64
	AssigneeID    int64
	Completed     string
	Archived      string
	Kind          string
	DueFrom       *time.Time
	DueTo         *time.Time
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	CompletedFrom *time.Time
	CompletedTo   *time.Time
	ArchivedFrom  *time.Time
	ArchivedTo    *time.Time
	Sort          string
	SortDesc      bool
	Limit         int32
	Offset        int32
}

type TaskCollaborantsParams struct {
	ViewerID  int64
	NameQuery string
	Limit     int32
	Offset    int32
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
	ParentID    *int64
	ParentTitle *string
	CreatedAt   time.Time
	CompletedAt *time.Time
	ArchivedAt  *time.Time
	IsArchived  bool
}

func NewCardRepository(db *pgxpool.Pool) *CardRepository {
	return &CardRepository{
		Db: db,
	}
}

// Архивность и дата архивации у подзадачи берутся у родителя: сама она
// не архивируется.
const (
	taskArchivedExpr   = `CASE WHEN c.parent_id IS NULL THEN c.is_archived ELSE COALESCE(parent.is_archived, FALSE) END`
	taskArchivedAtExpr = `CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END`
	taskPriorityRank   = `CASE c.priority WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END`
)

var taskListColumns = []string{
	// Окно считается до LIMIT — отдельный COUNT-запрос не нужен.
	"COUNT(*) OVER ()::bigint",
	"p.id", "p.name",
	"b.id", "b.title",
	"col.id", "col.title",
	"c.id", "c.title", "c.priority", "c.due_date", "c.border_color",
	"c.parent_id", "parent.title",
	"c.created_at", "c.completed_at",
	"(" + taskArchivedAtExpr + ")::timestamptz",
	"(" + taskArchivedExpr + ")::bool",
}

// taskListQuery собирает запрос только из активных фильтров. Прежний вариант
// прошивал каждый как «параметр пуст ИЛИ предикат»: планировщик строил один
// generic plan на все комбинации и всегда выбирал полный скан. Здесь он видит
// настоящие предикаты. Для assignee_id это вдобавок смена точки входа — JOIN
// по idx_kanban_card_assignee_user_id вместо EXISTS поверх скана всех карточек.
func taskListQuery(f TaskListParams) sq.SelectBuilder {
	q := sq.Select(taskListColumns...).
		PlaceholderFormat(sq.Dollar).
		From("kanban_card c").
		LeftJoin("kanban_card parent ON parent.id = c.parent_id").
		Join("kanban_column col ON col.id = COALESCE(c.column_id, parent.column_id)").
		Join("kanban_board b ON b.id = col.board_id").
		Join("kanban_project p ON p.id = b.kanban_project_id").
		Where("c.deleted_at IS NULL").
		Where("(parent.id IS NULL OR parent.deleted_at IS NULL)").
		Where("col.deleted_at IS NULL").
		Where("b.deleted_at IS NULL").
		Where("p.deleted_at IS NULL").
		Where(`(p.owner_id = ? OR EXISTS (
			SELECT 1 FROM kanban_project_user pu
			WHERE pu.kanban_project_id = p.id AND pu.user_id = ?
		))`, f.ViewerID, f.ViewerID)

	if f.TitleQuery != "" {
		// Спецсимволы LIKE экранированы ещё в обработчике, отсюда ESCAPE.
		q = q.Where(`c.title ILIKE '%' || ? || '%' ESCAPE '\'`, f.TitleQuery)
	}
	if f.ProjectID != 0 {
		q = q.Where(sq.Eq{"p.id": f.ProjectID})
	}
	switch {
	case f.Priority == "none":
		q = q.Where("c.priority IS NULL")
	case f.Priority != "":
		q = q.Where(sq.Eq{"c.priority": f.Priority})
	}
	switch {
	case f.AuthorID < 0:
		q = q.Where("c.created_by_id IS NULL")
	case f.AuthorID > 0:
		q = q.Where(sq.Eq{"c.created_by_id": f.AuthorID})
	}
	switch {
	case f.AssigneeID < 0:
		q = q.Where("NOT EXISTS (SELECT 1 FROM kanban_card_assignee ca WHERE ca.card_id = c.id)")
	case f.AssigneeID > 0:
		// PK (card_id, user_id) гарантирует не больше одной строки — дублей не будет.
		q = q.Join("kanban_card_assignee ca ON ca.card_id = c.id").
			Where(sq.Eq{"ca.user_id": f.AssigneeID})
	}
	switch f.Completed {
	case "true":
		q = q.Where("c.completed_at IS NOT NULL")
	case "false":
		q = q.Where("c.completed_at IS NULL")
	}
	switch f.Archived {
	case "true":
		q = q.Where("(" + taskArchivedExpr + ")")
	case "false":
		q = q.Where("NOT (" + taskArchivedExpr + ")")
	}
	switch f.Kind {
	case "task":
		q = q.Where("c.parent_id IS NULL")
	case "subtask":
		q = q.Where("c.parent_id IS NOT NULL")
	}

	for _, r := range []struct {
		expr string
		from *time.Time
		to   *time.Time
	}{
		{"c.due_date", f.DueFrom, f.DueTo},
		{"c.created_at", f.CreatedFrom, f.CreatedTo},
		{"c.completed_at", f.CompletedFrom, f.CompletedTo},
		{"(" + taskArchivedAtExpr + ")", f.ArchivedFrom, f.ArchivedTo},
	} {
		if r.from != nil {
			q = q.Where(r.expr+" >= ?", *r.from)
		}
		if r.to != nil {
			q = q.Where(r.expr+" < ?", *r.to)
		}
	}

	return q.OrderBy(taskListOrderBy(f.Sort, f.SortDesc)...)
}

// taskListOrderBy — белый список: sort выбирает ветку и никогда не попадает
// в SQL как текст, направление приходит из bool.
func taskListOrderBy(sort string, desc bool) []string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	switch sort {
	case "title":
		return []string{"c.title " + dir, "c.id ASC"}
	case "createdAt":
		return []string{"c.created_at " + dir, "c.id ASC"}
	case "completedAt":
		return []string{"c.completed_at " + dir + " NULLS LAST", "c.id ASC"}
	default: // priority — он же значение по умолчанию в обработчике
		return []string{taskPriorityRank + " " + dir, "c.id ASC"}
	}
}

func (r *CardRepository) queryTaskList(ctx context.Context, q sq.SelectBuilder, limit, offset int32) ([]AssignedCardRow, int64, error) {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	sqlStr, args, err := q.Limit(uint64(limit)).Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.Db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]AssignedCardRow, 0, limit)
	var total int64
	for rows.Next() {
		var row AssignedCardRow
		var priority, borderColor, parentTitle pgtype.Text
		var dueDate, createdAt, completedAt, archivedAt pgtype.Timestamptz
		var parentID pgtype.Int8

		if err := rows.Scan(
			&total,
			&row.ProjectID, &row.ProjectName,
			&row.BoardID, &row.BoardTitle,
			&row.ColumnID, &row.ColumnTitle,
			&row.CardID, &row.CardTitle, &priority, &dueDate, &borderColor,
			&parentID, &parentTitle,
			&createdAt, &completedAt,
			&archivedAt, &row.IsArchived,
		); err != nil {
			return nil, 0, err
		}

		row.Priority = textPtr(priority)
		row.DueDate = timestamptzPtr(dueDate)
		row.BorderColor = textPtr(borderColor)
		row.ParentID = int8Ptr(parentID)
		row.ParentTitle = textPtr(parentTitle)
		row.CreatedAt = createdAt.Time
		row.CompletedAt = timestamptzPtr(completedAt)
		row.ArchivedAt = timestamptzPtr(archivedAt)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *CardRepository) ListTasks(ctx context.Context, f TaskListParams) ([]AssignedCardRow, int64, error) {
	q := taskListQuery(f)
	rows, total, err := r.queryTaskList(ctx, q, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 && f.Offset > 0 {
		// Страница за пределами набора: окно считать не с чего, а отдать
		// total=0 нельзя — клиент потеряет пагинацию. Добираем одной строкой
		// с начала, и только в этой редкой ветке.
		if _, total, err = r.queryTaskList(ctx, q, 1, 0); err != nil {
			return nil, 0, err
		}
	}
	return rows, total, nil
}

func (r *CardRepository) ListTaskCollaborants(ctx context.Context, f TaskCollaborantsParams) ([]model.User, int64, error) {
	queries := dbgen.New(r.Db)
	params := dbgen.ListTaskCollaborantsParams{
		ViewerID:   f.ViewerID,
		NameQuery:  f.NameQuery,
		PageLimit:  f.Limit,
		PageOffset: f.Offset,
	}
	rows, err := queries.ListTaskCollaborants(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if f.Offset > 0 {
		probe := params
		probe.PageLimit, probe.PageOffset = 1, 0
		first, err := queries.ListTaskCollaborants(ctx, probe)
		if err != nil {
			return nil, 0, err
		}
		if len(first) > 0 {
			total = first[0].TotalCount
		}
	}

	result := make([]model.User, 0, len(rows))
	for _, row := range rows {
		result = append(result, model.User{
			ID:         row.ID,
			Login:      row.Login,
			Lastname:   row.Lastname,
			Firstname:  row.Firstname,
			Patronymic: textPtr(row.Patronymic),
			AvatarName: textPtr(row.AvatarName),
		})
	}
	return result, total, nil
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

func int8Arg(id int64) pgtype.Int8 {
	return pgtype.Int8{Int64: id, Valid: true}
}

func columnIDArg(c *model.Card) pgtype.Int8 {
	if c.ParentID != nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: c.ColumnID, Valid: true}
}

func parentIDArg(c *model.Card) pgtype.Int8 {
	if c.ParentID == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *c.ParentID, Valid: true}
}

func mapDBCard(c dbgen.KanbanCard) model.Card {
	card := model.Card{
		ID:         c.ID,
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		CreatedAt:  c.CreatedAt.Time,
		UpdatedAt:  c.UpdatedAt.Time,
	}
	if c.ColumnID.Valid {
		card.ColumnID = c.ColumnID.Int64
	}
	card.ParentID = int8Ptr(c.ParentID)
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
	return card
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
		WHERE col.board_id = $1 AND c.parent_id IS NULL AND c.is_archived = FALSE AND c.deleted_at IS NULL AND col.deleted_at IS NULL`

	var count int
	if err := r.Db.QueryRow(ctx, query, boardID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *CardRepository) GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetCardsByColumn(ctx, int8Arg(columnID))
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
		card := mapDBCard(c)
		card.AssigneeIDs = assigneesByCard[card.ID]
		card.LabelIDs = labelsByCard[card.ID]
		cards = append(cards, card)
	}
	return cards, nil
}

func (r *CardRepository) GetChildCards(ctx context.Context, parentID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetChildCards(ctx, int8Arg(parentID))
	if err != nil {
		return nil, err
	}
	if len(dbCards) == 0 {
		return []model.Card{}, nil
	}
	cardIDs := make([]int64, len(dbCards))
	for i, c := range dbCards {
		cardIDs[i] = c.ID
	}
	assigneesByCard, err := r.GetAssigneesByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	cards := make([]model.Card, 0, len(dbCards))
	for _, c := range dbCards {
		card := mapDBCard(c)
		card.AssigneeIDs = assigneesByCard[card.ID]
		cards = append(cards, card)
	}
	return cards, nil
}

func (r *CardRepository) MoveChildCard(ctx context.Context, id int64, position float64) (*model.CardMove, error) {
	row, err := dbgen.New(r.Db).UpdateCardPosition(ctx, dbgen.UpdateCardPositionParams{
		ID:       id,
		Position: position,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}
	return &model.CardMove{
		ID:           row.ID,
		Title:        row.Title,
		Position:     row.Position,
		FromPosition: row.OldPosition,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

func (r *CardRepository) GetChildStats(ctx context.Context, parentID int64) (total int64, maxPosition float64, err error) {
	row, err := dbgen.New(r.Db).GetChildStats(ctx, int8Arg(parentID))
	if err != nil {
		return 0, 0, err
	}
	return row.Total, row.MaxPosition, nil
}

func (r *CardRepository) GetChildCountsByParentIDs(ctx context.Context, parentIDs []int64) (map[int64]model.ChecklistCount, error) {
	result := make(map[int64]model.ChecklistCount)
	if len(parentIDs) == 0 {
		return result, nil
	}
	rows, err := dbgen.New(r.Db).GetChildCountsByParentIDs(ctx, parentIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !row.ParentID.Valid {
			continue
		}
		result[row.ParentID.Int64] = model.ChecklistCount{Total: int(row.Total), Done: int(row.Done)}
	}
	return result, nil
}

func (r *CardRepository) GetCardsByBoard(ctx context.Context, boardID int64) ([]model.Card, error) {
	queries := dbgen.New(r.Db)
	dbCards, err := queries.GetCardsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	var cards []model.Card
	for _, c := range dbCards {
		cards = append(cards, mapDBCard(c))
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
		ColumnID: columnIDArg(c),
		ParentID: parentIDArg(c),
	}
	if c.ParentID == nil && columnID != 0 {
		params.ColumnID = int8Arg(columnID)
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

	card := mapDBCard(c)
	cardPtr := &card

	// Bulk-запросы для assignees и labels (используем существующие методы)
	assigneesByCard, err := r.GetAssigneesByCardIDs(ctx, []int64{cardPtr.ID})
	if err != nil {
		return nil, err
	}
	cardPtr.AssigneeIDs = assigneesByCard[cardPtr.ID]

	labelsByCard, err := r.GetLabelIDsByCardIDs(ctx, []int64{cardPtr.ID})
	if err != nil {
		return nil, err
	}
	cardPtr.LabelIDs = labelsByCard[cardPtr.ID]

	return cardPtr, nil
}

func (r *CardRepository) UpdateCard(ctx context.Context, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.UpdateCardParams{
		Title:      c.Title,
		Position:   c.Position,
		IsArchived: c.IsArchived,
		ColumnID:   columnIDArg(c),
		ParentID:   parentIDArg(c),
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

// columnCardPositions — порядок колонки и ничего лишнего: GetCardsByColumn ради
// тех же двух полей делает ещё два запроса, за исполнителями и метками.
func (r *CardRepository) columnCardPositions(ctx context.Context, columnID int64) ([]model.CardPosition, error) {
	rows, err := r.Db.Query(ctx, `
		SELECT id, position, updated_at
		FROM kanban_card
		WHERE column_id = $1 AND parent_id IS NULL AND is_archived = FALSE AND deleted_at IS NULL
		ORDER BY position ASC, id ASC`, columnID)
	if err != nil {
		return nil, NormalizeError(err)
	}
	defer rows.Close()

	positions := make([]model.CardPosition, 0)
	for rows.Next() {
		var (
			p         model.CardPosition
			updatedAt pgtype.Timestamptz
		)
		if err := rows.Scan(&p.ID, &p.Position, &updatedAt); err != nil {
			return nil, err
		}
		p.UpdatedAt = updatedAt.Time
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return positions, nil
}

func (r *CardRepository) MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.CardMove, error) {
	move := &model.CardMove{ID: id, ToColumnID: columnID}

	// 1. Исходная колонка, позиция и заголовок — всё, что от карточки нужно
	// перемещению. Заголовок уедет в уведомление о смене колонки, позиция — в
	// историю как Before.
	var fromColumn pgtype.Int8
	if err := r.Db.QueryRow(ctx, `
		SELECT column_id, position, title FROM kanban_card WHERE id = $1
	`, id).Scan(&fromColumn, &move.FromPosition, &move.Title); err != nil {
		return nil, NormalizeError(err)
	}
	if fromColumn.Valid {
		move.FromColumnID = fromColumn.Int64
	}

	// 2. Занята ли целевая позиция — один EXISTS вместо чтения всей колонки.
	needsRebalance, err := dbgen.New(r.Db).ColumnPositionTaken(ctx, dbgen.ColumnPositionTakenParams{
		ColumnID: int8Arg(columnID),
		ID:       id,
		Position: position,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	// 3. Узкий UPDATE вместо перезаписи всей строки: перемещение больше не затирает
	// заголовок или описание, которые в этот же момент правит кто-то другой.
	var updatedAt pgtype.Timestamptz
	if err := r.Db.QueryRow(ctx, `
		UPDATE kanban_card
		SET column_id = $1, position = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING position, updated_at
	`, columnID, position, id).Scan(&move.Position, &updatedAt); err != nil {
		return nil, NormalizeError(err)
	}
	move.UpdatedAt = updatedAt.Time

	if !needsRebalance {
		return move, nil
	}

	// 4. Ребалансировка переписала позиции всей колонки — перечитываем, иначе
	// у клиента останутся устаревшие позиции соседних карточек.
	if err := dbgen.New(r.Db).RebalanceColumnCards(ctx, int8Arg(columnID)); err != nil {
		return nil, err
	}
	rebalanced, err := r.columnCardPositions(ctx, columnID)
	if err != nil {
		return nil, err
	}
	move.Rebalanced = rebalanced
	for _, p := range rebalanced {
		if p.ID == id {
			move.Position = p.Position
			break
		}
	}
	return move, nil
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
		SELECT ca.user_id
		FROM kanban_card_assignee ca
		JOIN kanban_card child ON child.id = ca.card_id
		WHERE child.parent_id = $1 AND child.deleted_at IS NULL
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
