package service

import (
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

func mapTaskListItems(rows []repository.AssignedCardRow) []*dto.TaskListItem {
	items := make([]*dto.TaskListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, &dto.TaskListItem{
			ID:          row.CardID,
			Title:       row.CardTitle,
			Priority:    row.Priority,
			DueDate:     row.DueDate,
			BorderColor: row.BorderColor,
			ParentID:    row.ParentID,
			ParentTitle: row.ParentTitle,
			Column:      dto.TaskRef{ID: row.ColumnID, Title: row.ColumnTitle},
			Board:       dto.TaskRef{ID: row.BoardID, Title: row.BoardTitle},
			Project:     dto.TaskProjectRef{ID: row.ProjectID, Name: row.ProjectName},
			CreatedAt:   row.CreatedAt,
			CompletedAt: row.CompletedAt,
			ArchivedAt:  row.ArchivedAt,
			IsArchived:  row.IsArchived,
		})
	}
	return items
}

func attachAssigneesToTaskItems(items []*dto.TaskListItem, byCard map[int64][]int64, users []model.User) {
	if len(items) == 0 {
		return
	}
	userByID := make(map[int64]model.User, len(users))
	for _, user := range users {
		userByID[user.ID] = user
	}
	for _, item := range items {
		ids := byCard[item.ID]
		if len(ids) == 0 {
			continue
		}
		if user, ok := userByID[ids[0]]; ok {
			item.Assignee = dto.MapUserResponse(&user)
		}
	}
}
