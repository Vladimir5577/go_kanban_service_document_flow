package events

import (
	"log/slog"

	"github.com/google/uuid"
)

// NewEventID — uuid v7 для поля EventID общего контракта.
//
// Версия 7, а не 4: она время-упорядоченная, и вставки в уникальный индекс
// консьюмера идут в хвост b-tree, а не вразброс по страницам. Размер тот же
// (16 байт), случайных бит даже меньше.
//
// Генерировать id надо при СОЗДАНИИ события и переиспользовать при повторной
// отправке: новый id на ретрае сделает дедупликацию бессмысленной.
func NewEventID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Сломан источник случайности. Пустой id консьюмер отвергнет — это
		// честнее, чем прислать событие, которое нечем дедуплицировать.
		slog.Error("не удалось сгенерировать eventId уведомления", "error", err)
		return ""
	}
	return id.String()
}

// KanbanNotificationEvent — уведомление, публикуемое в RabbitMQ по действию
// в канбане. Ключи вида "kanban.notification.<событие>".
//
// Поля общего контракта уведомлений (EventID, Title, TypeLabel, Message, Link)
// заполняет продюсер: только модуль знает формулировки своих событий. Раньше
// заголовки собирал switch внутри сервиса нотификаций, и любой новый тип
// события требовал правки и деплоя чужого сервиса.
//
// Спецификация контракта:
// go_notification_service_document_flow/TODO_generic_notifications.md
type KanbanNotificationEvent struct {
	// EventID — uuid v7, один на событие (не на получателя). По нему консьюмер
	// отсекает дубли, поэтому при повторной отправке id обязан остаться тем же.
	EventID string `json:"eventId"`

	// Title — готовый заголовок уведомления.
	Title string `json:"title"`

	// TypeLabel — подпись категории для списка уведомлений.
	TypeLabel string `json:"typeLabel,omitempty"`

	// Message — дополнительный текст. У канбана не используется.
	Message *string `json:"message,omitempty"`

	// Link — маршрут SPA, куда ведёт уведомление.
	Link *string `json:"link,omitempty"`

	// --- Ниже поля прежнего формата. Нужны только на время переходного
	// --- деплоя: пока в проде старый сервис нотификаций, он собирает
	// --- заголовок из Type и Data. После выката нового — удалить Type,
	// --- ProjectID, BoardID, CardID; Data остаётся, она уезжает в extra.

	// Type of the notification event. Examples:
	//   "card_created"
	//   "task_assigned"
	//   "task_moved"
	//   "comment_added"
	//   "subtask_assigned"
	//   "project_user_added"
	//   "project_user_removed"
	Type string `json:"type"`

	// ID of the user who performed the action (the actor).
	ActorID int64 `json:"actorId"`

	// Project this action belongs to (highly recommended for filtering).
	ProjectID int64 `json:"projectId"`

	// Optional context IDs.
	BoardID *int64 `json:"boardId,omitempty"`
	CardID  *int64 `json:"cardId,omitempty"`

	// List of user IDs that should receive this notification.
	// The Kanban service computes this list using the same rules as Symfony
	// (admins + assignees + subtask users, excluding the actor).
	Recipients []int64 `json:"recipients"`

	// Additional data needed to render the notification message.
	// Contents depend on Type. Common fields:
	//   cardTitle, taskTitle, title, boardTitle, authorName,
	//   fromColumnTitle, toColumnTitle, projectName, isSubtask, link, etc.
	//
	// boardTitle is enriched by KanbanNotificationService when board context
	// is available (via explicit boardID or resolved from card -> column).
	//
	// authorName is enriched here (lastname + firstname or login) to match
	// exactly how Symfony computes it before calling NotificationService:
	//   trim(lastname . ' ' . firstname) ?: login
	Data map[string]any `json:"data"`
}

// Routing key helpers (use these when publishing)
const (
	RoutingKanbanNotificationCardCreated        = "kanban.notification.card_created"
	RoutingKanbanNotificationTaskAssigned       = "kanban.notification.task_assigned"
	RoutingKanbanNotificationTaskMoved          = "kanban.notification.task_moved"
	RoutingKanbanNotificationCommentAdded       = "kanban.notification.comment_added"
	RoutingKanbanNotificationSubtaskAssigned    = "kanban.notification.subtask_assigned"
	RoutingKanbanNotificationProjectUserAdded   = "kanban.notification.project_user_added"
	RoutingKanbanNotificationProjectUserRemoved = "kanban.notification.project_user_removed"
)
