package dto

import (
	"encoding/json"
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

// MoveProjectRequest DTO для персонального перемещения проекта в сайдбаре.
type MoveProjectRequest struct {
	FolderID *int64   `json:"folderId,omitempty"`
	Position *float64 `json:"position" validate:"required"`
}

func (r *MoveProjectRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		FolderID      *int64   `json:"folderId"`
		FolderIDSnake *int64   `json:"folder_id"`
		Position      *float64 `json:"position"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.FolderID = raw.FolderID
	if r.FolderID == nil {
		r.FolderID = raw.FolderIDSnake
	}
	r.Position = raw.Position
	return nil
}

type MoveProjectResponse struct {
	ID                 int64                 `json:"id"`
	FolderID           *int64                `json:"folderId"`
	Position           float64               `json:"position"`
	RebalancedProjects []*NavProjectResponse `json:"rebalancedProjects"`
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
	EntryBoardId   *int64            `json:"entryBoardId,omitempty"`
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
		ID:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
		EntryBoardId: p.EntryBoardID,
		Boards:       make([]*BoardResponse, 0),
		Members:      make([]*MemberResponse, 0),
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

	// Create entry href: /kanban/projects/{id}
	// (updated to include 'kanban' suffix to match current application URLs after extraction)
	return &NavProjectResponse{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		IsOwner:        isOwner,
		IsProjectAdmin: isProjectAdmin,
		EntryBoardId:   p.EntryBoardID,
		EntryHref:      "/kanban/projects/" + formatID(p.ID),
		FolderId:       p.FolderID,
		Position:       p.Position,
	}
}

// Helper func
func formatID(id int64) string {
	return fmt.Sprintf("%d", id)
}
