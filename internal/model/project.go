package model

import "time"

// Project представляет проект канбана (таблица kanban_project).
type Project struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	OwnerID      int64      `json:"owner_id"`
	CreatedByID  *int64     `json:"created_by_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	EntryBoardID *int64     `json:"entryBoardId,omitempty"`
}

// NavProject представляет проект для сайдбара (объединение проекта, прав и позиции).
type NavProject struct {
	ID           int64
	Name         string
	Description  *string
	OwnerID      int64
	Role         string
	FolderID     *int64
	Position     float64
	EntryBoardID *int64
}
