package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type CreateCardRequest struct {
	Title       string     `json:"title" validate:"required,max=500"`
	ColumnID    int64      `json:"column_id" validate:"required"`
	Description *string    `json:"description,omitempty"`
	Position    *float64   `json:"position,omitempty"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
	Priority    *string    `json:"priority,omitempty"`
	BorderColor *string    `json:"borderColor,omitempty"`
	AssigneeIDs []int64    `json:"assignee_ids,omitempty"`
	LabelIDs    []int64    `json:"label_ids,omitempty"`
}

type UpdateCardRequest struct {
	Title       *string    `json:"title,omitempty" validate:"omitempty,max=500"`
	ColumnID    *int64     `json:"column_id,omitempty"`
	Description *string    `json:"description,omitempty"`
	Position    *float64   `json:"position,omitempty"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
	Priority    *string    `json:"priority,omitempty"`
	IsArchived  *bool      `json:"is_archived,omitempty"`
	BorderColor *string    `json:"borderColor,omitempty"`
	AssigneeIDs []int64    `json:"assignee_ids,omitempty"`
	LabelIDs    []int64    `json:"label_ids,omitempty"`
}

type CardAssigneeResponse struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	AvatarUrl *string `json:"avatarUrl,omitempty"`
}

type CardResponse struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Description   *string    `json:"description,omitempty"`
	Position      float64    `json:"position"`
	DueDate       *time.Time `json:"dueDate,omitempty"`
	Priority      *string    `json:"priority,omitempty"`
	IsArchived    bool       `json:"isArchived"`
	ArchivedAt    *time.Time `json:"archivedAt,omitempty"`
	ArchivedByID  *int64     `json:"archivedById,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	CompletedByID *int64     `json:"completedById,omitempty"`
	ColumnID      int64      `json:"columnId"`
	CreatedByID   *int64     `json:"createdById,omitempty"`
	BorderColor   *string    `json:"borderColor,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	AssigneeIDs   []int64    `json:"assigneeIds,omitempty"`
	LabelIDs      []int64    `json:"labelIds,omitempty"`
	Labels        []*LabelResponse        `json:"labels"`
	Assignees     []*CardAssigneeResponse `json:"assignees"`
	Comments      []*CommentResponse      `json:"comments"`
	Subtasks      []*SubtaskResponse      `json:"subtasks"`
	Attachments   []*AttachmentResponse   `json:"attachments"`
	ChecklistTotal int                     `json:"checklistTotal"`
	ChecklistDone  int                     `json:"checklistDone"`
	CommentsCount  int                     `json:"commentsCount"`
}

func MapCardResponse(c *model.Card) *CardResponse {
	if c == nil {
		return nil
	}
	return &CardResponse{
		ID:            c.ID,
		Title:         c.Title,
		Description:   c.Description,
		Position:      c.Position,
		DueDate:       c.DueDate,
		Priority:      c.Priority,
		IsArchived:    c.IsArchived,
		ArchivedAt:    c.ArchivedAt,
		ArchivedByID:  c.ArchivedByID,
		CompletedAt:   c.CompletedAt,
		CompletedByID: c.CompletedByID,
		ColumnID:      c.ColumnID,
		CreatedByID:   c.CreatedByID,
		BorderColor:   c.BorderColor,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
		AssigneeIDs:   c.AssigneeIDs,
		LabelIDs:      c.LabelIDs,
		Labels:        make([]*LabelResponse, 0),
		Assignees:     make([]*CardAssigneeResponse, 0),
		Comments:      make([]*CommentResponse, 0),
		Subtasks:      make([]*SubtaskResponse, 0),
		Attachments:   make([]*AttachmentResponse, 0),
	}
}

func MapCardsResponse(cards []model.Card) []*CardResponse {
	resp := make([]*CardResponse, 0, len(cards))
	for i := range cards {
		resp = append(resp, MapCardResponse(&cards[i]))
	}
	return resp
}
