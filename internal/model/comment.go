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

// CommentRead — отметка прочтения из kanban_card_read: заход пользователя,
// накрывший комментарий.
type CommentRead struct {
	UserID int64
	ReadAt time.Time
}

// CommentReader — человек, прочитавший комментарий. ReadBeforeEdit взводится,
// если комментарий правили после прочтения: тогда человек видел другой текст.
type CommentReader struct {
	User           User
	ReadAt         time.Time
	ReadBeforeEdit bool
}

// CommentReaders — содержимое модалки «кто видел»: прочитавшие и остальные
// участники проекта. Автор комментария не попадает ни в один из списков.
type CommentReaders struct {
	Readers []CommentReader
	Pending []User
}
