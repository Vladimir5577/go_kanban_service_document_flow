package model

import "time"

// Comment представляет комментарий-сообщение чата карточки
// (таблица kanban_card_comment).
type Comment struct {
	ID         int64      `json:"id"`
	Body       string     `json:"body"`
	CardID     int64      `json:"card_id"`
	AuthorID   int64      `json:"author_id"`
	AuthorName string     `json:"authorName"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}
