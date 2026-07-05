package dto

import (
	"fmt"
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

// NavProjectResponse DTO для элемента бокового меню (проекты пользователя)
type NavProjectResponse struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	IsOwner        bool    `json:"isOwner"`
	IsProjectAdmin bool    `json:"isProjectAdmin"`
	EntryBoardId   *int64  `json:"entryBoardId"`
	EntryHref      string  `json:"entryHref"`
	FolderId       *int64  `json:"folderId"`
	Position       float64 `json:"position"`
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

// MapNavProjectResponse конвертирует внутреннюю модель NavProject в DTO для сайдбара
func MapNavProjectResponse(p model.NavProject, currentUserID int64) *NavProjectResponse {
	isOwner := p.OwnerID == currentUserID
	isProjectAdmin := isOwner || p.Role == "KANBAN_ADMIN"
	
	// Create entry href: /projects/{id}
	// By default, frontend expects it to be the project root
	// If entry board is present, the frontend will navigate there automatically when clicking the nav item.
	// We just provide the project base URL.
	// Wait, the frontend code for EntryHref: entryHref: `/projects/${project.id}`
	
	// If EntryBoardID is null, we can just leave it as null, frontend handles it.
	
	// Helper function for float to float mapping
	// Or we just map it.
	return &NavProjectResponse{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		IsOwner:        isOwner,
		IsProjectAdmin: isProjectAdmin,
		EntryBoardId:   p.EntryBoardID,
		EntryHref:      "/projects/" + formatID(p.ID), // we need a small helper or just fmt.Sprintf
		FolderId:       p.FolderID,
		Position:       p.Position,
	}
}

// Helper func
func formatID(id int64) string {
	return fmt.Sprintf("%d", id)
}
