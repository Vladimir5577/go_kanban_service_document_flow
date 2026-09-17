package model

import "time"

// Card представляет карточку-задачу (таблица kanban_card).
//
// AssigneeIDs и LabelIDs — связи M2M (kanban_card_assignee, kanban_card_label).
// Отдельных моделей у таблиц-связок нет: это чистые id-id без собственных полей,
// поэтому они представлены срезами идентификаторов внутри карточки.
type Card struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Description   *string    `json:"description,omitempty"`
	Position      float64    `json:"position"`
	DueDate       *time.Time `json:"due_date,omitempty"`
	Priority      *string    `json:"priority,omitempty"`
	IsArchived    bool       `json:"is_archived"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	ArchivedByID  *int64     `json:"archived_by_id,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CompletedByID *int64     `json:"completed_by_id,omitempty"`
	ColumnID      int64      `json:"column_id"`
	ParentID      *int64     `json:"parent_id,omitempty"`
	CreatedByID   *int64     `json:"created_by_id,omitempty"`
	BorderColor   *string    `json:"border_color,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	AssigneeIDs []int64 `json:"assignee_ids,omitempty"`
	LabelIDs    []int64 `json:"label_ids,omitempty"`
}

// CardPosition — место карточки в колонке. Отдельный тип, потому что для порядка
// нужны два поля, а чтение полной Card тянет за собой ещё два запроса — за
// метками и исполнителями, которые к порядку отношения не имеют.
type CardPosition struct {
	ID        int64
	Position  float64
	UpdatedAt time.Time
}

// CardMove — результат перемещения карточки: только то, что при этом меняется.
// Rebalanced заполняется, лишь если перемещение вызвало перенумерацию колонки,
// иначе остаётся nil.
type CardMove struct {
	ID           int64
	Title        string
	FromColumnID int64
	FromPosition float64
	ToColumnID   int64
	Position     float64
	UpdatedAt    time.Time
	Rebalanced   []CardPosition
}

type BoardArchiveFilters struct {
	Title       string
	Description string
	DateFrom    string
	DateTo      string
	OrderBy     string // title | column | archived_at | created_at | completed_at
	Order       string // ASC | DESC
	Page        int
	Limit       int
}

type ArchivedCard struct {
	ID          int64
	Title       string
	Description *string
	ColumnTitle string
	BorderColor *string
	CreatedAt   time.Time
	ArchivedAt  *time.Time
	ArchivedBy  *User
	CompletedAt  *time.Time
	CompletedBy  *User
	Assignees   []User
}

type ChecklistCount struct {
	Total int
	Done  int
}

type BoardArchivePage struct {
	Cards         []ArchivedCard
	Page          int
	Limit         int
	Total         int64
	ArchivedCount int64
}
