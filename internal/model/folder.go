package model

import "time"

// ProjectUserFolder представляет папку группировки проектов в сайдбаре
// пользователя (таблица kanban_project_user_folder).
type ProjectUserFolder struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UserID    int64     `json:"user_id"`
	Position  float64   `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
