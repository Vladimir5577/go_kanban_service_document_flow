package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go_kanban_service/internal/messaging/events"
	"go_kanban_service/internal/repository"
)

// KanbanNotificationService centralizes the logic of deciding
// when and to whom to send Kanban notifications, and publishes
// them to RabbitMQ (to be consumed by a separate notification service).
//
// This mirrors the behavior that used to live directly inside
// Symfony's NotificationService calls in Kanban controllers.
type KanbanNotificationService struct {
	publisher         *events.Publisher
	projectMemberRepo repository.ProjectMemberRepositoryInterface
	cardRepo          repository.CardRepositoryInterface
	userRepo          repository.UserRepositoryInterface
}

func NewKanbanNotificationService(
	publisher *events.Publisher,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
) *KanbanNotificationService {
	return &KanbanNotificationService{
		publisher:         publisher,
		projectMemberRepo: projectMemberRepo,
		cardRepo:          cardRepo,
		userRepo:          userRepo,
	}
}

// getAuthorName enriches the actorID with a human-readable name
// (lastname + firstname, fallback to login).
// This replicates exactly how Symfony computes $authorName before calling NotificationService.
func (s *KanbanNotificationService) getAuthorName(ctx context.Context, actorID int64) string {
	if actorID == 0 || s.userRepo == nil {
		return ""
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, []int64{actorID})
	if err != nil || len(users) == 0 {
		return ""
	}

	u := users[0]
	name := strings.TrimSpace(u.Lastname + " " + u.Firstname)
	if name == "" {
		return u.Login
	}
	return name
}

// NotifyCardCreated notifies project admins about a new card (except the actor).
func (s *KanbanNotificationService) NotifyCardCreated(
	ctx context.Context,
	projectID, boardID, cardID int64,
	actorID int64,
	title, boardTitle string,
) {
	if s.publisher == nil {
		return
	}

	admins, err := s.projectMemberRepo.GetAdminUserIDs(ctx, projectID)
	if err != nil {
		slog.Warn("failed to get admin users for notification", "project_id", projectID, "error", err)
		return
	}

	actorPtr := int64Ptr(actorID)
	recipients := filterOutActor(admins, actorPtr)
	if len(recipients) == 0 {
		return
	}

	link := fmt.Sprintf("/projects/%d?board=%d&task=%d", projectID, boardID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "card_created",
		ActorID:    actorID,
		ProjectID:  projectID,
		BoardID:    &boardID,
		CardID:     &cardID,
		Recipients: recipients,
		Data: map[string]any{
			"cardTitle":  title,
			"boardTitle": boardTitle,
			"link":       link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationCardCreated, evt)
}

// NotifyTaskAssigned notifies a user that a task (or subtask) was assigned to them.
func (s *KanbanNotificationService) NotifyTaskAssigned(
	ctx context.Context,
	projectID, cardID int64,
	actorID, assigneeID int64,
	title string,
	isSubtask bool,
) {
	if s.publisher == nil || assigneeID == 0 || assigneeID == actorID {
		return
	}

	link := fmt.Sprintf("/projects/%d?task=%d", projectID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "task_assigned",
		ActorID:    actorID,
		ProjectID:  projectID,
		CardID:     &cardID,
		Recipients: []int64{assigneeID},
		Data: map[string]any{
			"title":     title,
			"isSubtask": isSubtask,
			"link":      link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationTaskAssigned, evt)
}

// NotifyTaskMoved notifies relevant users when a card is moved to another column.
func (s *KanbanNotificationService) NotifyTaskMoved(
	ctx context.Context,
	projectID, cardID int64,
	actorID int64,
	title, fromColumn, toColumn string,
) {
	if s.publisher == nil {
		return
	}

	involved, err := s.cardRepo.GetInvolvedUserIDsForNotifications(ctx, cardID)
	if err != nil {
		slog.Warn("failed to get involved users", "card_id", cardID, "error", err)
		return
	}

	admins, err := s.projectMemberRepo.GetAdminUserIDs(ctx, projectID)
	if err != nil {
		slog.Warn("failed to get admins", "project_id", projectID, "error", err)
		return
	}

	all := uniqueUserIDs(append(involved, admins...))
	recipients := filterOutActor(all, int64Ptr(actorID))
	if len(recipients) == 0 {
		return
	}

	authorName := s.getAuthorName(ctx, actorID)

	link := fmt.Sprintf("/projects/%d?task=%d", projectID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "task_moved",
		ActorID:    actorID,
		ProjectID:  projectID,
		CardID:     &cardID,
		Recipients: recipients,
		Data: map[string]any{
			"taskTitle":       title,
			"authorName":      authorName,
			"fromColumnTitle": fromColumn,
			"toColumnTitle":   toColumn,
			"link":            link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationTaskMoved, evt)
}

// NotifyCommentAdded notifies relevant users about a new comment.
func (s *KanbanNotificationService) NotifyCommentAdded(
	ctx context.Context,
	projectID, boardID, cardID int64,
	actorID int64,
	taskTitle string,
) {
	if s.publisher == nil {
		return
	}

	admins, _ := s.projectMemberRepo.GetAdminUserIDs(ctx, projectID)
	involved, _ := s.cardRepo.GetInvolvedUserIDsForNotifications(ctx, cardID)

	all := uniqueUserIDs(append(admins, involved...))
	recipients := filterOutActor(all, int64Ptr(actorID))
	if len(recipients) == 0 {
		return
	}

	authorName := s.getAuthorName(ctx, actorID)

	if taskTitle == "" {
		if card, err := s.cardRepo.GetCard(ctx, cardID); err == nil && card != nil {
			taskTitle = card.Title
		}
	}

	link := fmt.Sprintf("/projects/%d?board=%d&task=%d", projectID, boardID, cardID)
	if boardID == 0 {
		link = fmt.Sprintf("/projects/%d?task=%d", projectID, cardID)
	}

	evt := events.KanbanNotificationEvent{
		Type:       "comment_added",
		ActorID:    actorID,
		ProjectID:  projectID,
		CardID:     &cardID,
		Recipients: recipients,
		Data: map[string]any{
			"taskTitle":  taskTitle,
			"authorName": authorName,
			"link":       link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationCommentAdded, evt)
}

// NotifySubtaskAssigned is used when a subtask gets an assignee.
func (s *KanbanNotificationService) NotifySubtaskAssigned(
	ctx context.Context,
	projectID, cardID int64,
	actorID, assigneeID int64,
	title string,
) {
	if s.publisher == nil || assigneeID == 0 || assigneeID == actorID {
		return
	}

	// To match Symfony: subtaskTitle = title + ' (задача: ' + cardTitle + ')'
	if card, err := s.cardRepo.GetCard(ctx, cardID); err == nil && card != nil {
		title = title + " (задача: " + card.Title + ")"
	}

	link := fmt.Sprintf("/projects/%d?task=%d", projectID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "subtask_assigned",
		ActorID:    actorID,
		ProjectID:  projectID,
		CardID:     &cardID,
		Recipients: []int64{assigneeID},
		Data: map[string]any{
			"title":     title,
			"isSubtask": true,
			"link":      link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationSubtaskAssigned, evt)
}

// NotifyProjectUserAdded notifies a user that they were added to a project.
func (s *KanbanNotificationService) NotifyProjectUserAdded(
	ctx context.Context,
	projectID int64,
	actorID, newUserID int64,
	projectName string,
) {
	if s.publisher == nil || newUserID == 0 || newUserID == actorID {
		return
	}

	link := fmt.Sprintf("/projects/%d", projectID)
	evt := events.KanbanNotificationEvent{
		Type:       "project_user_added",
		ActorID:    actorID,
		ProjectID:  projectID,
		Recipients: []int64{newUserID},
		Data: map[string]any{
			"projectName": projectName,
			"link":        link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationProjectUserAdded, evt)
}

// NotifyProjectUserRemoved notifies a user that they were removed from a project.
func (s *KanbanNotificationService) NotifyProjectUserRemoved(
	ctx context.Context,
	projectID int64,
	actorID, removedUserID int64,
	projectName string,
) {
	if s.publisher == nil || removedUserID == 0 {
		return
	}

	link := fmt.Sprintf("/projects/%d", projectID)
	evt := events.KanbanNotificationEvent{
		Type:       "project_user_removed",
		ActorID:    actorID,
		ProjectID:  projectID,
		Recipients: []int64{removedUserID},
		Data: map[string]any{
			"projectName": projectName,
			"link":        link,
		},
	}

	s.publisher.PublishAsync(events.RoutingKanbanNotificationProjectUserRemoved, evt)
}

// Note: filterOutActor and uniqueUserIDs are defined in card_service.go (same package).

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
