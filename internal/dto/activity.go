package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type ActivityUserResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ActivityResponse struct {
	ID        int64                 `json:"id"`
	CardID    int64                 `json:"cardId"`
	Type      string                `json:"type"`
	Label     string                `json:"label"`
	Icon      string                `json:"icon"`
	OldValue  *string               `json:"oldValue,omitempty"`
	NewValue  *string               `json:"newValue,omitempty"`
	CreatedAt time.Time             `json:"createdAt"`
	User      *ActivityUserResponse `json:"user,omitempty"`
}

func getActivityLabel(activityType string) string {
	switch activityType {
	case "created":
		return "Задача создана"
	case "moved":
		return "Перемещена в другой столбец"
	case "renamed":
		return "Изменено название"
	case "description_changed":
		return "Изменено описание"
	case "priority_changed":
		return "Изменён приоритет"
	case "due_date_changed":
		return "Изменён срок"
	case "assignee_added":
		return "Назначен исполнитель"
	case "assignee_removed":
		return "Снят исполнитель"
	case "label_added":
		return "Добавлена метка"
	case "label_removed":
		return "Удалена метка"
	case "color_changed":
		return "Изменён цвет"
	case "comment_added":
		return "Добавлен комментарий"
	case "attachment_added":
		return "Добавлено вложение"
	case "attachment_removed":
		return "Удалено вложение"
	case "subtask_added":
		return "Добавлена подзадача"
	case "subtask_completed":
		return "Подзадача выполнена"
	case "subtask_reopened":
		return "Подзадача снова открыта"
	case "subtask_removed":
		return "Удалена подзадача"
	case "subtask_assigned":
		return "Назначен исполнитель подзадачи"
	case "subtask_unassigned":
		return "Снят исполнитель подзадачи"
	case "archived":
		return "Задача архивирована"
	case "restored":
		return "Задача восстановлена из архива"
	case "completed":
		return "Задача выполнена"
	case "reopened":
		return "Задача снова открыта"
	default:
		return "Обновил(а) задачу"
	}
}

func MapActivityResponse(a *model.Activity) *ActivityResponse {
	if a == nil {
		return nil
	}
	
	resp := &ActivityResponse{
		ID:        a.ID,
		CardID:    a.CardID,
		Type:      a.Type,
		Label:     getActivityLabel(a.Type),
		Icon:      "", // Frontend handles this
		OldValue:  a.OldValue,
		NewValue:  a.NewValue,
		CreatedAt: a.CreatedAt,
	}
	
	if a.UserID != nil && a.UserName != nil {
		resp.User = &ActivityUserResponse{
			ID:   *a.UserID,
			Name: *a.UserName,
		}
	}
	return resp
}

func MapActivitiesResponse(activities []model.Activity) []*ActivityResponse {
	resp := make([]*ActivityResponse, 0, len(activities))
	for i := range activities {
		resp = append(resp, MapActivityResponse(&activities[i]))
	}
	return resp
}

type UserResponse struct {
	ID         int64   `json:"id"`
	Login      string  `json:"login"`
	Lastname   string  `json:"lastname"`
	Firstname  string  `json:"firstname"`
	Patronymic *string `json:"patronymic,omitempty"`
	AvatarName *string `json:"avatarName,omitempty"`
}

func MapUserResponse(u *model.User) *UserResponse {
	if u == nil {
		return nil
	}
	return &UserResponse{
		ID:         u.ID,
		Login:      u.Login,
		Lastname:   u.Lastname,
		Firstname:  u.Firstname,
		Patronymic: u.Patronymic,
		AvatarName: u.AvatarName,
	}
}

func MapUsersResponse(users []model.User) []*UserResponse {
	resp := make([]*UserResponse, 0, len(users))
	for i := range users {
		resp = append(resp, MapUserResponse(&users[i]))
	}
	return resp
}
