package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const mercurePublishTimeout = 5 * time.Second

type KanbanRealtimePublisher struct {
	hubURL    string
	jwtSecret string
	client    *http.Client

	cardRepo       repository.CardRepositoryInterface
	columnRepo     repository.ColumnRepositoryInterface
	subtaskRepo    repository.SubtaskRepositoryInterface
	commentRepo    repository.CommentRepositoryInterface
	attachmentRepo repository.AttachmentRepositoryInterface
	labelRepo      repository.LabelRepositoryInterface
	userRepo       repository.UserRepositoryInterface
	cfg            *config.Config
}

func NewKanbanRealtimePublisher(
	hubURL string,
	jwtSecret string,
	cardRepo repository.CardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	subtaskRepo repository.SubtaskRepositoryInterface,
	commentRepo repository.CommentRepositoryInterface,
	attachmentRepo repository.AttachmentRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	cfg *config.Config,
) *KanbanRealtimePublisher {
	return &KanbanRealtimePublisher{
		hubURL:    strings.TrimSpace(hubURL),
		jwtSecret: jwtSecret,
		client: &http.Client{
			Timeout: mercurePublishTimeout,
		},
		cardRepo:       cardRepo,
		columnRepo:     columnRepo,
		subtaskRepo:    subtaskRepo,
		commentRepo:    commentRepo,
		attachmentRepo: attachmentRepo,
		labelRepo:      labelRepo,
		userRepo:       userRepo,
		cfg:            cfg,
	}
}

func (p *KanbanRealtimePublisher) PublishCardUpdated(ctx context.Context, boardID int64, card map[string]any, senderID int64) error {
	return p.publish(ctx, boardID, map[string]any{
		"type":     "card_updated",
		"card":     card,
		"senderId": senderID,
	})
}

func (p *KanbanRealtimePublisher) PublishCardCreated(ctx context.Context, boardID int64, card map[string]any, senderID int64) error {
	return p.publish(ctx, boardID, map[string]any{
		"type":     "card_created",
		"card":     card,
		"senderId": senderID,
	})
}

func (p *KanbanRealtimePublisher) PublishCardDeleted(ctx context.Context, boardID int64, cardID int64, senderID int64) error {
	return p.publish(ctx, boardID, map[string]any{
		"type":     "card_deleted",
		"cardId":   cardID,
		"senderId": senderID,
	})
}

func (p *KanbanRealtimePublisher) TryPublish(ctx context.Context, publish func(context.Context) error) {
	if p == nil || publish == nil {
		return
	}
	if err := publish(ctx); err != nil {
		slog.WarnContext(ctx, "failed to publish kanban realtime event", "error", err)
	}
}

func (p *KanbanRealtimePublisher) PublishCardPatch(ctx context.Context, card *model.Card, partial map[string]any, senderID int64) error {
	if card == nil {
		return nil
	}

	column, err := p.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return err
	}

	patch := map[string]any{
		"id": card.ID,
	}
	for key, value := range partial {
		patch[key] = value
	}
	if _, ok := patch["updatedAt"]; !ok {
		patch["updatedAt"] = formatRealtimeTimeValue(card.UpdatedAt)
	}

	return p.PublishCardUpdated(ctx, column.BoardID, patch, senderID)
}

func (p *KanbanRealtimePublisher) PublishCardPatchByID(ctx context.Context, cardID int64, partial map[string]any, senderID int64) error {
	card, err := p.cardRepo.GetCard(ctx, cardID)
	if err != nil {
		return err
	}
	return p.PublishCardPatch(ctx, card, partial, senderID)
}

func (p *KanbanRealtimePublisher) BuildCreatedCard(card *model.Card, column *model.Column) map[string]any {
	return map[string]any{
		"id":             card.ID,
		"borderColor":    card.BorderColor,
		"title":          card.Title,
		"description":    card.Description,
		"position":       card.Position,
		"priority":       card.Priority,
		"dueDate":        formatRealtimeTime(card.DueDate),
		"labels":         []map[string]any{},
		"assignees":      []map[string]any{},
		"checklistTotal": 0,
		"checklistDone":  0,
		"commentsCount":  0,
		"updatedAt":      nil,
		"completedAt":    formatRealtimeTime(card.CompletedAt),
		"status":         strconv.FormatInt(column.ID, 10),
	}
}

func (p *KanbanRealtimePublisher) BuildChecklistCounters(ctx context.Context, cardID int64) (map[string]any, error) {
	subtasks, err := p.subtaskRepo.GetSubtasks(ctx, cardID)
	if err != nil {
		return nil, err
	}

	done := 0
	for _, subtask := range subtasks {
		if subtask.Status == "done" || subtask.Status == "DONE" {
			done++
		}
	}

	return map[string]any{
		"checklistTotal": len(subtasks),
		"checklistDone":  done,
	}, nil
}

func (p *KanbanRealtimePublisher) BuildCommentsCount(ctx context.Context, cardID int64) (map[string]any, error) {
	comments, err := p.commentRepo.GetComments(ctx, cardID)
	if err != nil {
		return nil, err
	}
	chatAttachments, err := p.attachmentRepo.GetAttachmentsByCard(ctx, cardID, "chat")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"commentsCount": len(comments) + len(chatAttachments),
	}, nil
}

func (p *KanbanRealtimePublisher) BuildLabels(ctx context.Context, cardID int64) (map[string]any, error) {
	card, err := p.cardRepo.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	column, err := p.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return nil, err
	}
	labels, err := p.labelRepo.GetLabels(ctx, column.BoardID)
	if err != nil {
		return nil, err
	}

	labelByID := make(map[int64]model.Label, len(labels))
	for _, label := range labels {
		labelByID[label.ID] = label
	}

	result := make([]map[string]any, 0, len(card.LabelIDs))
	for _, labelID := range card.LabelIDs {
		if label, ok := labelByID[labelID]; ok {
			result = append(result, map[string]any{
				"id":    label.ID,
				"name":  label.Name,
				"color": label.Color,
			})
		}
	}

	return map[string]any{
		"labels": result,
	}, nil
}

func (p *KanbanRealtimePublisher) BuildAssignees(ctx context.Context, cardID int64) (map[string]any, error) {
	card, err := p.cardRepo.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	users, err := p.userRepo.GetUsersByIDs(ctx, card.AssigneeIDs)
	if err != nil {
		return nil, err
	}

	userByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		userByID[user.ID] = user
	}

	assignees := make([]map[string]any, 0, len(card.AssigneeIDs))
	for _, userID := range card.AssigneeIDs {
		if user, ok := userByID[userID]; ok {
			assignees = append(assignees, formatRealtimeAssignee(p.cfg, user))
		}
	}

	return map[string]any{
		"assignees": assignees,
	}, nil
}

func (p *KanbanRealtimePublisher) publish(ctx context.Context, boardID int64, event map[string]any) error {
	if p == nil {
		return nil
	}
	if p.hubURL == "" {
		return fmt.Errorf("mercure hub url is not configured")
	}
	if p.jwtSecret == "" {
		return fmt.Errorf("mercure jwt secret is not configured")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	form := url.Values{}
	form.Set("topic", "/kanban/board/"+strconv.FormatInt(boardID, 10))
	form.Set("data", string(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.hubURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token, err := p.publisherToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("publish mercure update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("publish mercure update: status %d: %s", resp.StatusCode, string(bytes.TrimSpace(body)))
	}

	return nil
}

func (p *KanbanRealtimePublisher) publisherToken() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"mercure": map[string]any{
			"publish": []string{"*"},
		},
	})
	return token.SignedString([]byte(p.jwtSecret))
}

func formatRealtimeAssignee(cfg *config.Config, user model.User) map[string]any {
	name := strings.TrimSpace(user.Lastname + " " + user.Firstname)
	if name == "" {
		name = strconv.FormatInt(user.ID, 10)
	}

	return map[string]any{
		"id":        user.ID,
		"name":      name,
		"avatarUrl": dto.UserAvatarURL(cfg, user.AvatarName, dto.AvatarSizeThumbnail),
	}
}

func formatRealtimeTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339)
}

func formatRealtimeTimeValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339)
}

func realtimeSenderID(ctx context.Context) int64 {
	userID := currentUserID(ctx)
	if userID == nil {
		return 0
	}
	return *userID
}
