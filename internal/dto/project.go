package dto

import (
	"encoding/json"
	"fmt"
	"time"

	"go_kanban_service/internal/model"
)

// CreateProjectRequest DTO для создания проекта
type CreateProjectRequest struct {
	Name        string  `json:"name" validate:"required,min=3,max=70"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=1000"`
}

// UpdateProjectRequest DTO для обновления проекта
type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=3,max=70"`
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

// ProjectListItemResponse — строка справочника проектов.
type ProjectListItemResponse struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Description  *string       `json:"description,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	MembersCount int64         `json:"membersCount"`
	BoardsCount  int64         `json:"boardsCount"`
	TasksCount   int64         `json:"tasksCount"`
	Status       string        `json:"status"` // active | deleted
	DeletedAt    *time.Time    `json:"deletedAt,omitempty"`
	Owner        *UserResponse `json:"owner,omitempty"`
}

type ProjectListPaginationResponse struct {
	CurrentPage int   `json:"currentPage"`
	TotalPages  int   `json:"totalPages"`
	Total       int64 `json:"total"`
	Limit       int   `json:"limit"`
}

// ProjectListResponse — ответ GET /spa/api/kanban/projects (справочник).
type ProjectListResponse struct {
	Projects   []*ProjectListItemResponse    `json:"projects"`
	Pagination ProjectListPaginationResponse `json:"pagination"`
}

func MapProjectListResponse(page *model.ProjectListPage) *ProjectListResponse {
	if page == nil {
		return &ProjectListResponse{
			Projects: make([]*ProjectListItemResponse, 0),
		}
	}

	totalPages := 0
	if page.Limit > 0 && page.Total > 0 {
		totalPages = int((page.Total + int64(page.Limit) - 1) / int64(page.Limit))
	}

	items := make([]*ProjectListItemResponse, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, MapProjectListItemResponse(&page.Items[i]))
	}

	return &ProjectListResponse{
		Projects: items,
		Pagination: ProjectListPaginationResponse{
			CurrentPage: page.Page,
			TotalPages:  totalPages,
			Total:       page.Total,
			Limit:       page.Limit,
		},
	}
}

func MapProjectListItemResponse(item *model.ProjectListItem) *ProjectListItemResponse {
	if item == nil {
		return nil
	}
	status := model.ProjectStatusActive
	if item.DeletedAt != nil {
		status = model.ProjectStatusDeleted
	}
	return &ProjectListItemResponse{
		ID:           item.ID,
		Name:         item.Name,
		Description:  item.Description,
		CreatedAt:    item.CreatedAt,
		MembersCount: item.MembersCount,
		BoardsCount:  item.BoardsCount,
		TasksCount:   item.TasksCount,
		Status:       status,
		DeletedAt:    item.DeletedAt,
		Owner:        MapUserResponse(item.Owner),
	}
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
