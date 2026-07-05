package dto

import (
	"encoding/json"

	"go_kanban_service/internal/model"
)

type ReplaceProjectMembersRequest struct {
	Members []AddProjectMemberRequest `json:"members"`
}

type AddProjectMemberRequest struct {
	UserID   int64  `json:"user_id" validate:"required"`
	Role     string `json:"role" validate:"required"`
	FolderID *int64 `json:"folder_id,omitempty"`
}

func (r *AddProjectMemberRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		UserID        int64  `json:"user_id"`
		UserIDCamel   int64  `json:"userId"`
		Role          string `json:"role"`
		FolderID      *int64 `json:"folder_id"`
		FolderIDCamel *int64 `json:"folderId"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.UserID = raw.UserID
	if r.UserID == 0 {
		r.UserID = raw.UserIDCamel
	}
	r.Role = raw.Role
	r.FolderID = raw.FolderID
	if r.FolderID == nil {
		r.FolderID = raw.FolderIDCamel
	}
	return nil
}

type UpdateProjectMemberRequest struct {
	Role     *string  `json:"role,omitempty"`
	FolderID *int64   `json:"folder_id,omitempty"`
	Position *float64 `json:"position,omitempty"`
}

type ProjectMemberResponse struct {
	ID              int64   `json:"id"`
	KanbanProjectID int64   `json:"kanbanProjectId"`
	UserID          int64   `json:"userId"`
	Role            string  `json:"role"`
	FolderID        *int64  `json:"folderId,omitempty"`
	Position        float64 `json:"position"`
}

func MapProjectMemberResponse(m *model.ProjectUser) *ProjectMemberResponse {
	if m == nil {
		return nil
	}
	return &ProjectMemberResponse{
		ID:              m.ID,
		KanbanProjectID: m.KanbanProjectID,
		UserID:          m.UserID,
		Role:            m.Role,
		FolderID:        m.FolderID,
		Position:        m.Position,
	}
}

func MapProjectMembersResponse(members []model.ProjectUser) []*ProjectMemberResponse {
	resp := make([]*ProjectMemberResponse, 0, len(members))
	for i := range members {
		resp = append(resp, MapProjectMemberResponse(&members[i]))
	}
	return resp
}
