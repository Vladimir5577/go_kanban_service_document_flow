package events

// KanbanNotificationEvent is the payload published to RabbitMQ when a Kanban
// action that should generate a user notification occurs.
//
// The separate notification service is expected to consume these events
// (routing keys starting with "kanban.notification.") and create records
// in its own database.
//
// This mirrors the logic that was previously done directly via
// Symfony's NotificationService inside Kanban controllers.
type KanbanNotificationEvent struct {
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
	RoutingKanbanNotificationCardCreated     = "kanban.notification.card_created"
	RoutingKanbanNotificationTaskAssigned    = "kanban.notification.task_assigned"
	RoutingKanbanNotificationTaskMoved       = "kanban.notification.task_moved"
	RoutingKanbanNotificationCommentAdded    = "kanban.notification.comment_added"
	RoutingKanbanNotificationSubtaskAssigned = "kanban.notification.subtask_assigned"
	RoutingKanbanNotificationProjectUserAdded   = "kanban.notification.project_user_added"
	RoutingKanbanNotificationProjectUserRemoved = "kanban.notification.project_user_removed"
)
