package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type CreateProjectFolderRequest struct {
	Name     string   `json:"name" validate:"required,max=100"`
	Position *float64 `json:"position,omitempty"`
}

type UpdateProjectFolderRequest struct {
	Name     *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	Position *float64 `json:"position,omitempty"`
}

type ProjectFolderResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UserID    int64     `json:"userId"`
	Position  float64   `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func MapProjectFolderResponse(f *model.ProjectUserFolder) *ProjectFolderResponse {
	if f == nil {
		return nil
	}
	return &ProjectFolderResponse{
		ID:        f.ID,
		Name:      f.Name,
		UserID:    f.UserID,
		Position:  f.Position,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

func MapProjectFoldersResponse(folders []model.ProjectUserFolder) []*ProjectFolderResponse {
	resp := make([]*ProjectFolderResponse, 0, len(folders))
	for i := range folders {
		resp = append(resp, MapProjectFolderResponse(&folders[i]))
	}
	return resp
}
