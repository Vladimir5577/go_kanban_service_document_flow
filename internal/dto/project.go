package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

// CreateProjectRequest DTO для создания проекта
type CreateProjectRequest struct {
	Name        string  `json:"name" validate:"required,min=3,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// UpdateProjectRequest DTO для обновления проекта
type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=3,max=255"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
}


// MemberResponse DTO для участника проекта
type MemberResponse struct {
	UserID     int64   `json:"userId"`
	Login      string  `json:"login"`
	Lastname   string  `json:"lastname"`
	Firstname  string  `json:"firstname"`
	Patronymic *string `json:"patronymic,omitempty"`
	Profession *string `json:"profession,omitempty"`
	AvatarUrl  *string `json:"avatarUrl,omitempty"`
	Role       string  `json:"role"`
	RoleLabel  *string `json:"roleLabel,omitempty"`
	IsOwner    bool    `json:"isOwner"`
}

// ProjectResponse DTO для детального ответа клиенту (Fat API / BFF)
type ProjectResponse struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Description    *string           `json:"description,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
	Owner          *UserResponse     `json:"owner,omitempty"`
	IsOwner        bool              `json:"isOwner"`
	IsProjectAdmin bool              `json:"isProjectAdmin"`
	MemberRole     string            `json:"memberRole"`
	Boards         []*BoardResponse  `json:"boards"`
	Members        []*MemberResponse `json:"members"`
}

// MapProjectResponse конвертирует базовую модель в DTO (без связей)
func MapProjectResponse(p *model.Project) *ProjectResponse {
	if p == nil {
		return nil
	}
	return &ProjectResponse{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		Boards:      make([]*BoardResponse, 0),
		Members:     make([]*MemberResponse, 0),
	}
}

// MapProjectsResponse конвертирует срез моделей в срез DTO
func MapProjectsResponse(projects []model.Project) []*ProjectResponse {
	resp := make([]*ProjectResponse, 0, len(projects))
	for i := range projects {
		resp = append(resp, MapProjectResponse(&projects[i]))
	}
	return resp
}
