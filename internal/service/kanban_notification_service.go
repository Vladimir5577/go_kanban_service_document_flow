package service

import (
	"context"
	"fmt"
	"log/slog"

	"go_kanban_service/internal/dto"
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
	boardRepo         repository.BoardRepositoryInterface
	columnRepo        repository.ColumnRepositoryInterface
}

func NewKanbanNotificationService(
	publisher *events.Publisher,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	boardRepo repository.BoardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
) *KanbanNotificationService {
	return &KanbanNotificationService{
		publisher:         publisher,
		projectMemberRepo: projectMemberRepo,
		cardRepo:          cardRepo,
		userRepo:          userRepo,
		boardRepo:         boardRepo,
		columnRepo:        columnRepo,
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
	name := dto.UserDisplayName(u)
	if name == "" {
		return u.Login
	}
	return name
}

// getBoardTitle returns the human-readable title of a board.
// Mirrors the enrichment style of getAuthorName.
func (s *KanbanNotificationService) getBoardTitle(ctx context.Context, boardID int64) string {
	if boardID == 0 || s.boardRepo == nil {
		return ""
	}

	board, err := s.boardRepo.GetBoard(ctx, boardID)
	if err != nil || board == nil {
		return ""
	}
	return board.Title
}

// getBoardTitleForCard resolves the board title for a card by walking
// card -> column -> board. Used as a fallback when boardID is not directly available.
func (s *KanbanNotificationService) getBoardTitleForCard(ctx context.Context, cardID int64) string {
	if cardID == 0 || s.cardRepo == nil || s.columnRepo == nil {
		return ""
	}

	card, err := s.cardRepo.GetCard(ctx, cardID)
	if err != nil || card == nil {
		return ""
	}

	column, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil || column == nil {
		return ""
	}

	return s.getBoardTitle(ctx, column.BoardID)
}

// resolveBoardID returns the best available boardID.
// Prefers the explicitly passed boardID, otherwise resolves it from the card.
func (s *KanbanNotificationService) resolveBoardID(ctx context.Context, boardID, cardID int64) int64 {
	if boardID != 0 {
		return boardID
	}
	if cardID == 0 || s.cardRepo == nil || s.columnRepo == nil {
		return 0
	}

	card, err := s.cardRepo.GetCard(ctx, cardID)
	if err != nil || card == nil {
		return 0
	}

	column, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil || column == nil {
		return 0
	}
	return column.BoardID
}

// projectTaskLink — SPA permalink: /projects/:id/board-:boardId/task-:taskId.
func projectTaskLink(projectID, boardID, cardID int64) string {
	return fmt.Sprintf("/projects/%d/board-%d/task-%d", projectID, boardID, cardID)
}

// NotifyCardCreated notifies project admins about a new card (except the actor).
// boardTitle — если вызывающий уже знает его из проверки прав; пустой — сходим в GetBoard.
func (s *KanbanNotificationService) NotifyCardCreated(
	ctx context.Context,
	projectID, boardID int64,
	boardTitle string,
	cardID int64,
	actorID int64,
	title string,
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

	if boardTitle == "" {
		boardTitle = s.getBoardTitle(ctx, boardID)
	}

	link := projectTaskLink(projectID, boardID, cardID)
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

	s.publish(events.RoutingKanbanNotificationCardCreated, evt,
		fmt.Sprintf("Новая задача «%s» на доске «%s»", title, boardTitle),
		"Создана задача", link)
}

// NotifyTaskAssigned notifies a user that a task (or subtask) was assigned to them.
// boardTitle — если вызывающий уже знает его из проверки прав; пустой — сходим в GetBoard.
func (s *KanbanNotificationService) NotifyTaskAssigned(
	ctx context.Context,
	projectID, boardID int64,
	boardTitle string,
	cardID int64,
	actorID, assigneeID int64,
	title string,
	isSubtask bool,
) {
	if s.publisher == nil || assigneeID == 0 || assigneeID == actorID {
		return
	}

	resolvedBoardID := s.resolveBoardID(ctx, boardID, cardID)
	if boardTitle == "" {
		boardTitle = s.getBoardTitle(ctx, resolvedBoardID)
	}

	link := projectTaskLink(projectID, resolvedBoardID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "task_assigned",
		ActorID:    actorID,
		ProjectID:  projectID,
		BoardID:    int64Ptr(resolvedBoardID),
		CardID:     &cardID,
		Recipients: []int64{assigneeID},
		Data: map[string]any{
			"title":      title,
			"boardTitle": boardTitle,
			"isSubtask":  isSubtask,
			"link":       link,
		},
	}

	notifTitle := fmt.Sprintf("Вам назначена задача: %s", title)
	if isSubtask {
		notifTitle = fmt.Sprintf("Вам назначена подзадача: %s", title)
	}

	s.publish(events.RoutingKanbanNotificationTaskAssigned, evt,
		notifTitle, "Назначена задача", link)
}

// NotifyTaskMoved notifies relevant users when a card is moved to another column.
// boardTitle приходит от вызывающего: он получил его тем же запросом, которым
// проверял права, и лезть за ним в GetBoard больше незачем.
func (s *KanbanNotificationService) NotifyTaskMoved(
	ctx context.Context,
	projectID, boardID int64,
	boardTitle string,
	cardID int64,
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
	resolvedBoardID := s.resolveBoardID(ctx, boardID, cardID)
	if boardTitle == "" {
		boardTitle = s.getBoardTitle(ctx, resolvedBoardID)
	}

	link := projectTaskLink(projectID, resolvedBoardID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "task_moved",
		ActorID:    actorID,
		ProjectID:  projectID,
		BoardID:    int64Ptr(resolvedBoardID),
		CardID:     &cardID,
		Recipients: recipients,
		Data: map[string]any{
			"taskTitle":       title,
			"authorName":      authorName,
			"boardTitle":      boardTitle,
			"fromColumnTitle": fromColumn,
			"toColumnTitle":   toColumn,
			"link":            link,
		},
	}

	s.publish(events.RoutingKanbanNotificationTaskMoved, evt,
		fmt.Sprintf("%s переместил(а) задачу %s из колонки «%s» в колонку «%s»",
			authorName, title, fromColumn, toColumn),
		"Задача перемещена", link)
}

// NotifyCommentAdded notifies relevant users about a new comment.
// Заголовок задачи, доску и имя автора вызывающий уже знает.
func (s *KanbanNotificationService) NotifyCommentAdded(
	ctx context.Context,
	projectID, boardID int64,
	boardTitle string,
	cardID int64,
	actorID int64,
	taskTitle, authorName string,
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

	if authorName == "" {
		authorName = s.getAuthorName(ctx, actorID) // пустое ФИО — getAuthorName подставит логин
	}

	link := projectTaskLink(projectID, boardID, cardID)

	evt := events.KanbanNotificationEvent{
		Type:       "comment_added",
		ActorID:    actorID,
		ProjectID:  projectID,
		BoardID:    int64Ptr(boardID),
		CardID:     &cardID,
		Recipients: recipients,
		Data: map[string]any{
			"taskTitle":  taskTitle,
			"boardTitle": boardTitle,
			"authorName": authorName,
			"link":       link,
		},
	}

	s.publish(events.RoutingKanbanNotificationCommentAdded, evt,
		fmt.Sprintf("%s оставил(а) комментарий к задаче %s", authorName, taskTitle),
		"Новый комментарий в задаче", link)
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

	boardID := s.resolveBoardID(ctx, 0, cardID)
	boardTitle := s.getBoardTitle(ctx, boardID)

	link := projectTaskLink(projectID, boardID, cardID)
	evt := events.KanbanNotificationEvent{
		Type:       "subtask_assigned",
		ActorID:    actorID,
		ProjectID:  projectID,
		BoardID:    int64Ptr(boardID),
		CardID:     &cardID,
		Recipients: []int64{assigneeID},
		Data: map[string]any{
			"title":      title,
			"boardTitle": boardTitle,
			"isSubtask":  true,
			"link":       link,
		},
	}

	s.publish(events.RoutingKanbanNotificationSubtaskAssigned, evt,
		fmt.Sprintf("Вам назначена подзадача: %s", title),
		"Назначена задача", link)
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

	s.publish(events.RoutingKanbanNotificationProjectUserAdded, evt,
		fmt.Sprintf("Вас добавили в проект «%s»", projectName),
		"Добавлен в проект", link)
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

	s.publish(events.RoutingKanbanNotificationProjectUserRemoved, evt,
		fmt.Sprintf("Вас исключили из проекта «%s»", projectName),
		"Исключён из проекта", link)
}

// publish дополняет событие полями общего контракта и отправляет его.
//
// Заголовок и подпись приходят с места вызова: там уже есть и названия задач,
// и имена авторов, ради которых прежде городился switch на стороне сервиса
// нотификаций — в другом репозитории и с отдельным деплоем на каждый новый тип.
func (s *KanbanNotificationService) publish(
	routingKey string,
	evt events.KanbanNotificationEvent,
	title, typeLabel, link string,
) {
	evt.EventID = events.NewEventID()
	evt.Title = title
	evt.TypeLabel = typeLabel
	if link != "" {
		evt.Link = &link
	}

	s.publisher.PublishAsync(routingKey, evt)
}

// Note: filterOutActor and uniqueUserIDs are defined in card_service.go (same package).

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
