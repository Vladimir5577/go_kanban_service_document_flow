package dto

import "go_kanban_service/internal/model"

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
