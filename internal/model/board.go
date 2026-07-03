package model

import "time"

// Board представляет доску канбана (таблица kanban_board).
type Board struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`
	Position        float64    `json:"position"`
	KanbanProjectID int64      `json:"kanban_project_id"`
	CreatedByID     int64      `json:"created_by_id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}
