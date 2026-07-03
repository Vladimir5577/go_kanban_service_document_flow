package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type ActivityResponse struct {
	ID        int64     `json:"id"`
	CardID    int64     `json:"cardId"`
	UserID    *int64    `json:"userId,omitempty"`
	Type      string    `json:"type"`
	OldValue  *string   `json:"oldValue,omitempty"`
	NewValue  *string   `json:"newValue,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func MapActivityResponse(a *model.Activity) *ActivityResponse {
	if a == nil {
		return nil
	}
	return &ActivityResponse{
		ID:        a.ID,
		CardID:    a.CardID,
		UserID:    a.UserID,
		Type:      a.Type,
		OldValue:  a.OldValue,
		NewValue:  a.NewValue,
		CreatedAt: a.CreatedAt,
	}
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
