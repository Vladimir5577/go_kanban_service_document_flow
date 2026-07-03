package model

import "time"

// User представляет локальную реплику пользователя в базе данных канбана.
// Синхронизируется из Symfony через события RabbitMQ.
type User struct {
	ID         int64      `json:"id"`
	Login      string     `json:"login"`
	Lastname   string     `json:"lastname"`
	Firstname  string     `json:"firstname"`
	Patronymic *string    `json:"patronymic,omitempty"`
	AvatarName *string    `json:"avatar_name,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}
