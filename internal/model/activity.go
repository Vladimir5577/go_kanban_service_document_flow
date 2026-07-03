package model

import "time"

// Activity представляет запись истории действий по карточке
// (таблица kanban_card_activity).
type Activity struct {
	ID        int64     `json:"id"`
	CardID    int64     `json:"card_id"`
	UserID    *int64    `json:"user_id,omitempty"`
	Type      string    `json:"type"`
	OldValue  *string   `json:"old_value,omitempty"`
	NewValue  *string   `json:"new_value,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
