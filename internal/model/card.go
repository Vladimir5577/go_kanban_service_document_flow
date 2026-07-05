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
	CreatedByID   *int64     `json:"created_by_id,omitempty"`
	BorderColor   *string    `json:"border_color,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	AssigneeIDs []int64 `json:"assignee_ids,omitempty"`
	LabelIDs    []int64 `json:"label_ids,omitempty"`
}

type BoardArchiveFilters struct {
	Title       string
	Description string
	DateFrom    string
	DateTo      string
	Page        int
	Limit       int
}

type ArchivedCard struct {
	ID          int64
	Title       string
	Description *string
	ColumnTitle string
	BorderColor *string
	ArchivedAt  *time.Time
	ArchivedBy  *User
}

type BoardArchivePage struct {
	Cards         []ArchivedCard
	Page          int
	Limit         int
	Total         int64
	ArchivedCount int64
}
