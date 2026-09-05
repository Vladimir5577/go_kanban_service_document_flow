package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CardRepositoryInterface interface {
	CreateCard(ctx context.Context, in CreateCardInput) (*CreateCardResult, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardsByColumn(ctx context.Context, columnID int64) ([]model.Card, error)
	GetCardsByBoard(ctx context.Context, boardID int64) ([]model.Card, error)
	GetAssignedCards(ctx context.Context, userID int64, status string) ([]AssignedCardRow, error)
	GetAssignedSubtasks(ctx context.Context, userID int64, status string) ([]AssignedSubtaskRow, error)
	GetAssigneesByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	GetLabelIDsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64][]int64, error)
	// UpdateCardFields пишет только содержимое карточки (заголовок, описание,
	// срок, приоритет, цвет). Колонку/позицию, архив и «выполнено» меняют
	// свои методы — см. GK-02 в аудите 2026-09-05.
	UpdateCardFields(ctx context.Context, c *model.Card) (*model.Card, error)
	// SetCardCompletion ставит или снимает отметку «выполнено» атомарно:
	// expectOpen — ожидаемое текущее состояние (true = карточка ещё открыта).
	// Если состояние уже изменил кто-то другой — apperr.CodeConflict.
	SetCardCompletion(ctx context.Context, id int64, completedAt *time.Time, completedByID *int64, expectOpen bool) (*model.Card, error)
	// ArchiveCard уводит карточку в архив, не трогая остальные поля.
	ArchiveCard(ctx context.Context, id int64, archivedAt time.Time, archivedByID *int64) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error
	MoveCard(ctx context.Context, id int64, columnID int64, position float64, opts MoveCardOptions) (*MoveCardResult, error)
	// RestoreCard возвращает карточку из архива; отдаёт позиции колонки, если
	// при этом пришлось её перенумеровать.
	RestoreCard(ctx context.Context, id int64, boardID int64, maxActiveCards int) ([]CardPosition, error)

	// GetInvolvedUserIDsForNotifications returns distinct user IDs that are assignees on the card,
	// assignees on any of its subtasks, or the card's author. Used to decide notification recipients.
	GetInvolvedUserIDsForNotifications(ctx context.Context, cardID int64) ([]int64, error)
}

type CardRepository struct {
	Db *pgxpool.Pool

	// Точки синхронизации для интеграционных тестов. В рабочей сборке всегда
	// nil: их выставляют только тесты пакета, и только на своём экземпляре
	// репозитория.
	//
	// Раньше это были переменные уровня пакета; так дешевле, но любой тест мог
	// незаметно повлиять на соседний, а появление t.Parallel() превратило бы
	// это в гонку. Поле экземпляра такой возможности не оставляет.
	//
	// testHookAfterFirstColumnLock вызывается в MoveCard сразу после взятия
	// ПЕРВОЙ блокировки колонки — именно в этом промежутке видно, в каком
	// порядке транзакции берут ресурсы.
	testHookAfterFirstColumnLock func()

	// testHookBeforeCardInsert вызывается в CreateCard после проверки предела
	// и расчёта позиции, но до вставки строки.
	testHookBeforeCardInsert func()
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

// CreateCardInput — всё, что нужно для создания карточки одной транзакцией.
//
// Позиции здесь нет намеренно: её считает сервер, внутри транзакции, под
// блокировкой колонки. См. комментарий к CreateCard.
type CreateCardInput struct {
	BoardID  int64
	ColumnID int64
	Card     *model.Card

	// MaxActiveCards — предел активных карточек на доске. Передаётся из
	// сервиса, чтобы значение жило в одном месте, а проверялось там, где его
	// можно проверить честно, — внутри транзакции.
	MaxActiveCards int
}

// CardPosition — пара «карточка → позиция» после перенумерации колонки.
type CardPosition struct {
	ID       int64   `json:"id"`
	Position float64 `json:"position"`
}

// CreateCardResult — созданная карточка и, если колонку пришлось
// перенумеровать, позиции ВСЕХ её активных карточек.
//
// Ребаланс меняет позиции у всех карточек колонки, а клиенты раньше узнавали
// только о новой (GK-01): их следующий drag считал середины от устаревших
// чисел, карточка вставала не туда и снова провоцировала ребаланс.
type CreateCardResult struct {
	Card       *model.Card
	Rebalanced []CardPosition
}

// CompletionChange — что сделать с отметкой «выполнено» при переносе.
//
// Колонка «сделано» (board.done_column_id) до сих пор не имела серверной
// семантики (GK-03): перенос в неё не ставил completed_at, а вынос из неё не
// снимал — и два источника «выполнено» расходились. Решение принимает сервис,
// а пишется оно тем же UPDATE, что column_id/position, — без второго окна
// между «уже в колонке» и «ещё не выполнено».
type CompletionChange int

const (
	// CompletionKeep — отметку не трогать.
	CompletionKeep CompletionChange = iota
	// CompletionSet — поставить completed_at/completed_by_id (перенос в done-колонку).
	CompletionSet
	// CompletionClear — снять отметку (перенос из done-колонки).
	CompletionClear
)

type MoveCardOptions struct {
	Completion    CompletionChange
	CompletedAt   time.Time
	CompletedByID *int64
}

// MoveCardResult — перенесённая карточка и позиции колонки после ребаланса
// (пустой срез — ребаланса не было).
type MoveCardResult struct {
	Card       *model.Card
	Rebalanced []CardPosition
	// Completion — что реально применили: CompletionSet/Clear из opts
	// сводятся к Keep, если под блокировкой карточка уже в нужном состоянии
	// (параллельный перенос успел раньше).
	Completion CompletionChange
}

// minCardPosition — граница, ниже которой позиции больше не делятся пополам.
//
// Каждая карточка, добавленная в начало колонки, получает половину позиции
// верхней. Через несколько десятков добавлений подряд числа становятся
// неразличимо малы, а потом схлопываются в ноль, и порядок карточек
// перестаёт быть определённым. Дойдя до границы, колонка пересчитывается.
const minCardPosition = 1.0

// defaultCardPosition — позиция первой карточки в пустой колонке. Совпадает с
// шагом ребаланса, чтобы после пересчёта числа выглядели одинаково.
const defaultCardPosition = 65536.0

// CreateCard создаёт карточку вместе со связями.
//
// Исполнители и метки принимаются в запросе и раньше молча терялись: строка
// карточки вставлялась, а kanban_card_assignee и kanban_card_label не
// трогались вовсе. При этом сервис успевал разослать уведомления «вам
// назначена задача» по списку из памяти — то есть пользователь получал
// уведомление о назначении, которого в базе не было.
//
// Всё пишется одной транзакцией: карточка без своих связей — это не
// «частично созданная карточка», а карточка с потерянными данными.
//
// В той же транзакции — проверка предела карточек на доске и расчёт позиции.
// Раньше и то и другое считалось до транзакции: два одновременных создания
// видели одинаковую картину, и предел в 300 карточек обходился (обе вставки
// проходили при 299), а позиция «половина верхней» вычислялась из снимка,
// который к моменту вставки успевал устареть — карточки получали одинаковые
// позиции.
//
// Порядок взятия блокировок: сначала доска, потом колонка. Он должен быть
// единым во всём сервисе, иначе встречные операции образуют петлю ожидания.
// MoveCard блокирует только колонки, поэтому пересечения с ним нет: создание
// держит доску и ждёт колонку, перенос доску не трогает вовсе.
func (r *CardRepository) CreateCard(ctx context.Context, in CreateCardInput) (*CreateCardResult, error) {
	c := in.Card
	var rebalanced []CardPosition

	params := dbgen.CreateCardParams{
		Title:    c.Title,
		ColumnID: in.ColumnID,
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

	err := ExecTxWith(ctx, r.Db, func(tx pgx.Tx, q *dbgen.Queries) error {
		var lockedBoardID int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM kanban_board WHERE id = $1 FOR UPDATE`,
			in.BoardID,
		).Scan(&lockedBoardID); err != nil {
			return err
		}

		var lockedColumnID int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM kanban_column WHERE id = $1 FOR UPDATE`,
			in.ColumnID,
		).Scan(&lockedColumnID); err != nil {
			return err
		}

		// Предел карточек считается здесь, а не в сервисе: только под
		// блокировкой доски ответ на «сколько сейчас активных» остаётся
		// верным до самой вставки.
		var activeCards int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(c.id)
			   FROM kanban_card c
			   JOIN kanban_column col ON col.id = c.column_id
			  WHERE col.board_id = $1 AND c.is_archived = FALSE`,
			in.BoardID,
		).Scan(&activeCards); err != nil {
			return err
		}
		if in.MaxActiveCards > 0 && activeCards >= in.MaxActiveCards {
			return apperr.New(
				apperr.CodeBoardCardLimitReached,
				fmt.Sprintf("maximum number of cards (%d) on board reached", in.MaxActiveCards),
			)
		}

		position, didRebalance, err := nextTopPosition(ctx, tx, q, in.ColumnID)
		if err != nil {
			return err
		}
		c.Position = position
		params.Position = position

		// Точка синхронизации для интеграционных тестов: в рабочей сборке nil.
		// Стоит там, где раньше проходила граница транзакции: предел и позиция
		// уже посчитаны, вставки ещё не было. Именно в этом промежутке две
		// параллельные попытки создать карточку раньше видели одинаковую
		// картину.
		if r.testHookBeforeCardInsert != nil {
			r.testHookBeforeCardInsert()
		}

		res, err := q.CreateCard(ctx, params)
		if err != nil {
			return err
		}

		// Дубликаты в списке отсеиваем до вставки. У связующих таблиц
		// составной первичный ключ, поэтому assignee_ids вида [5, 5] дали бы
		// нарушение уникальности и откат всей транзакции — карточка не
		// создалась бы вовсе из-за того, что клиент прислал один и тот же id
		// дважды.
		for _, userID := range dedupeIDs(c.AssigneeIDs) {
			if err := q.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{
				CardID: res.ID,
				UserID: userID,
			}); err != nil {
				return err
			}
		}

		for _, labelID := range dedupeIDs(c.LabelIDs) {
			if err := q.AddCardLabel(ctx, dbgen.AddCardLabelParams{
				KanbanCardID:  res.ID,
				KanbanLabelID: labelID,
			}); err != nil {
				return err
			}
		}

		c.ID = res.ID
		c.CreatedAt = res.CreatedAt.Time
		c.UpdatedAt = res.UpdatedAt.Time

		// Позиции собираются в той же транзакции: между коммитом и
		// перечитыванием успел бы вклиниться чужой перенос, и клиент получил
		// бы третий вариант порядка.
		if didRebalance {
			rebalanced, err = columnCardPositions(ctx, q, in.ColumnID)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	return &CreateCardResult{Card: c, Rebalanced: rebalanced}, nil
}

// RestoreCard возвращает карточку из архива, соблюдая предел активных карточек
// на доске.
//
// Возврат из архива — вторая операция, которая увеличивает число активных
// карточек, и раньше она шла мимо предела: обычная запись строки без подсчёта и
// без блокировки. Доску с 300 карточками можно было раздуть сколько угодно —
// заархивировать одну, создать новую, вернуть архивную, повторить. Создание
// после этого отказывало, а доска уже была за пределом, и вернуть её в норму
// можно было только удалением.
//
// Проверка и запись идут в одной транзакции под блокировкой доски — той же, что
// в CreateCard, и в том же порядке. Поэтому одновременные «создать» и «вернуть
// из архива» выстраиваются в очередь, а не проходят оба по одному свободному
// месту.
func (r *CardRepository) RestoreCard(ctx context.Context, id int64, boardID int64, maxActiveCards int) ([]CardPosition, error) {
	var rebalanced []CardPosition
	err := ExecTxWith(ctx, r.Db, func(tx pgx.Tx, q *dbgen.Queries) error {
		var lockedBoardID int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM kanban_board WHERE id = $1 FOR UPDATE`,
			boardID,
		).Scan(&lockedBoardID); err != nil {
			return err
		}

		var activeCards int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(c.id)
			   FROM kanban_card c
			   JOIN kanban_column col ON col.id = c.column_id
			  WHERE col.board_id = $1 AND c.is_archived = FALSE`,
			boardID,
		).Scan(&activeCards); err != nil {
			return err
		}
		if maxActiveCards > 0 && activeCards >= maxActiveCards {
			return apperr.New(
				apperr.CodeBoardCardLimitReached,
				fmt.Sprintf("maximum number of cards (%d) on board reached", maxActiveCards),
			)
		}

		// Колонка карточки нужна дважды: заблокировать её (порядок тот же, что
		// в создании: доска, затем колонка) и посчитать позицию.
		var columnID int64
		if err := tx.QueryRow(ctx,
			`SELECT column_id FROM kanban_card WHERE id = $1 AND is_archived = TRUE`,
			id,
		).Scan(&columnID); err != nil {
			return err
		}

		var lockedColumnID int64
		if err := tx.QueryRow(ctx,
			`SELECT id FROM kanban_column WHERE id = $1 FOR UPDATE`,
			columnID,
		).Scan(&lockedColumnID); err != nil {
			return err
		}

		// Карточка возвращается наверх колонки с заново посчитанной позицией, а
		// не со старой.
		//
		// Расчёт позиций и ребаланс смотрят только на активные карточки, то
		// есть архивную они не видят и её место могут занять. Возврат со старым
		// значением давал две активные карточки на одной позиции: колонка с
		// единственной карточкой на 65536 архивируется, новая карточка получает
		// те же 65536, архивная возвращается — и порядок в колонке перестаёт
		// быть определённым. То же со старыми позициями вида 0 или
		// отрицательной, оставшимися от прежней арифметики.
		position, didRebalance, err := nextTopPosition(ctx, tx, q, columnID)
		if err != nil {
			return err
		}

		if r.testHookBeforeCardInsert != nil {
			r.testHookBeforeCardInsert()
		}

		// Пишутся ровно те поля, которые меняет возврат из архива. Общий
		// UpdateCard переписал бы всю строку значениями из снимка,
		// прочитанного до транзакции, и потерял бы параллельные правки
		// заголовка или срока.
		//
		// Условие по column_id страхует от переноса карточки, случившегося
		// между чтением колонки и взятием блокировки: тогда возврат честно
		// сообщит «не найдено», а не запишет позицию из чужой колонки.
		tag, err := tx.Exec(ctx,
			`UPDATE kanban_card
			    SET is_archived = FALSE, archived_at = NULL, archived_by_id = NULL,
			        position = $2, updated_at = NOW()
			  WHERE id = $1 AND is_archived = TRUE AND column_id = $3`,
			id, position, columnID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// Либо карточки нет, либо её уже вернул кто-то другой, пока мы
			// ждали блокировку. Второе не ошибка данных, но и молча делать
			// вид, что вернули именно мы, нельзя.
			return apperr.ErrNotFound
		}

		if didRebalance {
			rebalanced, err = columnCardPositions(ctx, q, columnID)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return rebalanced, NormalizeError(err)
}

// columnCardPositions — позиции активных карточек колонки внутри транзакции.
func columnCardPositions(ctx context.Context, q *dbgen.Queries, columnID int64) ([]CardPosition, error) {
	rows, err := q.GetColumnCardPositions(ctx, columnID)
	if err != nil {
		return nil, err
	}
	positions := make([]CardPosition, 0, len(rows))
	for _, row := range rows {
		positions = append(positions, CardPosition{ID: row.ID, Position: row.Position})
	}
	return positions, nil
}

// nextTopPosition считает позицию карточки, добавляемой в начало колонки.
//
// Новая карточка встаёт над верхней и получает половину её позиции — тот же
// расчёт, что делает фронт при перетаскивании в начало списка. Вычитать
// фиксированный шаг нельзя: на первой же вставке он уводит позицию в ноль и
// ниже, и порядок ломается.
//
// Вызывать только внутри транзакции, в которой колонка уже заблокирована:
// иначе между чтением верхней позиции и вставкой успевает вклиниться чужая
// карточка, и обе получат одинаковое значение.
//
// Второе значение — пришлось ли перенумеровать колонку: тогда позиции
// изменились у всех карточек, и вызывающий обязан сообщить об этом клиентам.
func nextTopPosition(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, columnID int64) (float64, bool, error) {
	top, err := minActivePosition(ctx, tx, columnID)
	if err != nil {
		return 0, false, err
	}
	if top == nil {
		return defaultCardPosition, false, nil
	}
	if half := *top / 2.0; half >= minCardPosition {
		return half, false, nil
	}

	// Делить дальше нечего: позиции стали неразличимо малы. Раскладываем
	// колонку заново с обычным шагом — порядок карточек при этом сохраняется,
	// меняются только числа.
	if err := q.RebalanceColumnCards(ctx, columnID); err != nil {
		return 0, false, err
	}

	top, err = minActivePosition(ctx, tx, columnID)
	if err != nil {
		return 0, true, err
	}
	if top == nil {
		return defaultCardPosition, true, nil
	}
	return *top / 2.0, true, nil
}

// minActivePosition отдаёт позицию верхней активной карточки колонки или nil,
// если колонка пуста.
func minActivePosition(ctx context.Context, tx pgx.Tx, columnID int64) (*float64, error) {
	var position *float64
	if err := tx.QueryRow(ctx,
		`SELECT MIN(position) FROM kanban_card WHERE column_id = $1 AND is_archived = FALSE`,
		columnID,
	).Scan(&position); err != nil {
		return nil, err
	}
	return position, nil
}

func (r *CardRepository) GetCard(ctx context.Context, id int64) (*model.Card, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetCard(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	card := cardFromRow(c)

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

// cardFromRow — модель из строки kanban_card (без исполнителей и меток).
func cardFromRow(c dbgen.KanbanCard) *model.Card {
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
	return card
}

// UpdateCardFields пишет содержимое карточки — и только его.
//
// Раньше здесь был общий UpdateCard, переписывавший всю строку из снимка,
// прочитанного до записи (GK-02): правка описания в момент чужого переноса
// возвращала карточку в прежнюю колонку, а параллельная отметка «выполнено»
// затиралась NULL-ом. Колонку, позицию, архив и «выполнено» этот запрос не
// трогает — им занимаются MoveCard, ArchiveCard/RestoreCard и SetCardCompletion.
func (r *CardRepository) UpdateCardFields(ctx context.Context, c *model.Card) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.UpdateCardFieldsParams{
		ID:    c.ID,
		Title: c.Title,
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

	res, err := queries.UpdateCardFields(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	// Возвращаем актуальную строку целиком: realtime-патч и ответ должны
	// нести настоящие column_id/position/completed_at, а не снимок.
	updated := cardFromRow(res)
	updated.AssigneeIDs = c.AssigneeIDs
	updated.LabelIDs = c.LabelIDs
	return updated, nil
}

// SetCardCompletion — атомарный toggle отметки «выполнено».
//
// expectOpen — состояние, которое видел вызывающий. Если к моменту записи
// его уже изменил другой пользователь, строка не совпадёт с условием и
// вернётся CodeConflict, а не «карточка не найдена»: фронту нужно перечитать
// карточку, а не показывать «удалена» на живой карточке.
func (r *CardRepository) SetCardCompletion(ctx context.Context, id int64, completedAt *time.Time, completedByID *int64, expectOpen bool) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.SetCardCompletionParams{ID: id, ExpectOpen: expectOpen}
	if completedAt != nil {
		params.CompletedAt = pgtype.Timestamptz{Time: *completedAt, Valid: true}
	}
	if completedByID != nil {
		params.CompletedByID = pgtype.Int8{Int64: *completedByID, Valid: true}
	}

	res, err := queries.SetCardCompletion(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "card completion changed concurrently")
		}
		return nil, NormalizeError(err)
	}
	return cardFromRow(res), nil
}

// ArchiveCard уводит карточку в архив точечным UPDATE. Повторная архивация
// уже архивной карточки — конфликт, а не тихая перезапись archived_at.
func (r *CardRepository) ArchiveCard(ctx context.Context, id int64, archivedAt time.Time, archivedByID *int64) (*model.Card, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.ArchiveCardRowParams{
		ID:         id,
		ArchivedAt: pgtype.Timestamptz{Time: archivedAt, Valid: true},
	}
	if archivedByID != nil {
		params.ArchivedByID = pgtype.Int8{Int64: *archivedByID, Valid: true}
	}

	res, err := queries.ArchiveCardRow(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeConflict, "card already archived")
		}
		return nil, NormalizeError(err)
	}
	return cardFromRow(res), nil
}

func (r *CardRepository) DeleteCard(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteCard(ctx, id)
}

// UpdateCardAssignees заменяет набор исполнителей карточки.
//
// Операция «очистить и записать заново» обязана быть атомарной: сбой между
// ClearCardAssignees и вставками оставлял карточку вообще без исполнителей,
// хотя пользователь просто менял одного на другого.
func (r *CardRepository) UpdateCardAssignees(ctx context.Context, cardID int64, userIDs []int64) error {
	return ExecTx(ctx, r.Db, func(q *dbgen.Queries) error {
		if err := q.ClearCardAssignees(ctx, cardID); err != nil {
			return err
		}

		for _, uid := range dedupeIDs(userIDs) {
			if err := q.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{
				CardID: cardID,
				UserID: uid,
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

// dedupeIDs убирает повторы, сохраняя порядок первого вхождения.
func dedupeIDs(ids []int64) []int64 {
	if len(ids) < 2 {
		return ids
	}

	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))

	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	return unique
}

// MoveCard переносит карточку в колонку на заданную позицию.
//
// Раньше это были четыре независимых запроса без транзакции: прочитать
// карточку, прочитать содержимое колонки, проверить коллизию позиций,
// записать. Два одновременных перетаскивания в одну колонку успевали оба
// увидеть «коллизии нет» и записать почти одинаковые позиции, а ребаланс мог
// идти параллельно со вставкой — порядок карточек после перезагрузки
// становился непредсказуемым.
//
// Теперь проверка и запись выполняются в одной транзакции, а колонка-приёмник
// блокируется: параллельные перемещения в неё выстраиваются в очередь.
//
// Отметка «выполнено» (opts.Completion) пишется тем же UPDATE, что колонка и
// позиция: перенос в done-колонку и completed_at не должны расходиться даже
// на мгновение (GK-03). Позиции после ребаланса собираются внутри транзакции
// и отдаются в результате (GK-01).
func (r *CardRepository) MoveCard(ctx context.Context, id int64, columnID int64, position float64, opts MoveCardOptions) (*MoveCardResult, error) {
	const epsilon = 0.0001
	var rebalanced []CardPosition
	appliedCompletion := CompletionKeep

	err := ExecTxWith(ctx, r.Db, func(tx pgx.Tx, q *dbgen.Queries) error {
		// Блокируем ОБЕ колонки — исходную и целевую — и обязательно в
		// порядке возрастания id.
		//
		// Одной целевой колонки мало. Встречные переносы A→B и B→A блокируют
		// разные колонки, спокойно расходятся дальше и упираются друг в друга
		// уже на строках карточек: ребаланс приёмника трогает карточку,
		// которую вторая транзакция как раз уводит к себе. Postgres в такой
		// ситуации снимает одну из транзакций с 40P01. Единый порядок взятия
		// блокировок делает такую петлю невозможной: вторая транзакция ждёт
		// уже на первой колонке, не успев тронуть ни одной строки карточек.
		var sourceColumnID int64
		if err := tx.QueryRow(ctx,
			`SELECT column_id FROM kanban_card WHERE id = $1`,
			id,
		).Scan(&sourceColumnID); err != nil {
			return err
		}

		lockOrder := []int64{sourceColumnID, columnID}
		if lockOrder[0] > lockOrder[1] {
			lockOrder[0], lockOrder[1] = lockOrder[1], lockOrder[0]
		}
		// Точка синхронизации тестов стоит между первой и второй блокировкой:
		// только в этом промежутке видно, в каком порядке транзакции берут
		// колонки. Если ждать позже, обе успевают взять оба ресурса
		// поодиночке, петля не складывается, и тест зеленеет на коде, в
		// котором порядок нарушен.
		meet := r.testHookAfterFirstColumnLock
		if meet == nil {
			meet = func() {}
		}

		locks := dedupeIDs(lockOrder)
		for i, lockColumnID := range locks {
			var lockedColumnID int64
			if err := tx.QueryRow(ctx,
				`SELECT id FROM kanban_column WHERE id = $1 FOR UPDATE`,
				lockColumnID,
			).Scan(&lockedColumnID); err != nil {
				return err
			}
			if i == 0 {
				meet()
			}
		}
		// Если блокировок не осталось вовсе, встреча всё равно должна
		// состояться: иначе тест, снявший защиту целиком, потеряет вместе с
		// ней и чередование — и станет зелёным именно там, где обязан падать.
		if len(locks) == 0 {
			meet()
		}

		// Решение «поставить/снять отметку» пересчитываем по заблокированной
		// строке, а не по снимку до транзакции: параллельный перенос мог уже
		// завершить или вернуть карточку в работу (GK-03).
		var lockedCompletedAt *time.Time
		if err := tx.QueryRow(ctx,
			`SELECT completed_at FROM kanban_card WHERE id = $1 FOR UPDATE`,
			id,
		).Scan(&lockedCompletedAt); err != nil {
			return err
		}
		applied := opts.Completion
		switch {
		case applied == CompletionSet && lockedCompletedAt != nil:
			applied = CompletionKeep
		case applied == CompletionClear && lockedCompletedAt == nil:
			applied = CompletionKeep
		}
		appliedCompletion = applied

		siblings, err := q.GetCardsByColumn(ctx, columnID)
		if err != nil {
			return err
		}

		needsRebalance := false
		for _, sibling := range siblings {
			if sibling.ID != id && math.Abs(sibling.Position-position) < epsilon {
				needsRebalance = true
				break
			}
		}

		// Пишем ровно те два поля, которые меняет перемещение.
		//
		// Раньше здесь вызывался общий UpdateCard, переписывающий всю строку
		// значениями из снимка, прочитанного ДО транзакции. Это давало
		// потерянное обновление: если параллельно кто-то переименовывал
		// карточку, перемещение возвращало заголовок к прежнему значению.
		var (
			tag pgconn.CommandTag
		)
		switch applied {
		case CompletionSet:
			var completedBy pgtype.Int8
			if opts.CompletedByID != nil {
				completedBy = pgtype.Int8{Int64: *opts.CompletedByID, Valid: true}
			}
			tag, err = tx.Exec(ctx,
				`UPDATE kanban_card
				    SET column_id = $1, position = $2, updated_at = NOW(),
				        completed_at = $4, completed_by_id = $5
				  WHERE id = $3`,
				columnID, position, id, opts.CompletedAt, completedBy,
			)
		case CompletionClear:
			tag, err = tx.Exec(ctx,
				`UPDATE kanban_card
				    SET column_id = $1, position = $2, updated_at = NOW(),
				        completed_at = NULL, completed_by_id = NULL
				  WHERE id = $3`,
				columnID, position, id,
			)
		default:
			tag, err = tx.Exec(ctx,
				`UPDATE kanban_card
				    SET column_id = $1, position = $2, updated_at = NOW()
				  WHERE id = $3`,
				columnID, position, id,
			)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.ErrNotFound
		}

		if needsRebalance {
			if err := q.RebalanceColumnCards(ctx, columnID); err != nil {
				return err
			}
			// Срез позиций — из той же транзакции: после коммита между ним и
			// перечитыванием успел бы вклиниться чужой перенос.
			rebalanced, err = columnCardPositions(ctx, q, columnID)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	// Карточка перечитывается после коммита всегда, а не только после
	// ребаланса: только так вернётся строка со всеми полями в актуальном
	// состоянии, включая изменения, сделанные параллельно.
	card, err := r.GetCard(ctx, id)
	if err != nil {
		return nil, err
	}
	return &MoveCardResult{Card: card, Rebalanced: rebalanced, Completion: appliedCompletion}, nil
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
