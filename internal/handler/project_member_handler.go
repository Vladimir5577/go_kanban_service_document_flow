package handler

import (
	"encoding/json"
	"net/http"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type ProjectMemberHandler struct {
	service    service.ProjectMemberServiceInterface
	projectSvc service.ProjectServiceInterface
}

func NewProjectMemberHandler(s service.ProjectMemberServiceInterface, projectSvc service.ProjectServiceInterface) *ProjectMemberHandler {
	return &ProjectMemberHandler{service: s, projectSvc: projectSvc}
}

func (h *ProjectMemberHandler) ReplaceMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		reqs, err := decodeReplaceMembersRequest(raw)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		if err := h.service.ReplaceMembers(r.Context(), projectID, reqs); err != nil {
			helper.WriteError(w, err)
			return
		}

		project, err := h.projectSvc.GetProject(r.Context(), projectID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{"members": project.Members})
	}
}

func decodeReplaceMembersRequest(raw json.RawMessage) ([]dto.AddProjectMemberRequest, error) {
	var wrapped dto.ReplaceProjectMembersRequest
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Members != nil {
		return wrapped.Members, nil
	}

	var reqs []dto.AddProjectMemberRequest
	if err := json.Unmarshal(raw, &reqs); err == nil {
		return reqs, nil
	}

	return nil, apperr.New(apperr.CodeMembersArrayExpected, "members array expected")
}

func (h *ProjectMemberHandler) UpdateMemberRole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		userID, err := helper.IDParam(r, "userId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateProjectMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if err := h.service.UpdateMemberRole(r.Context(), projectID, userID, req); err != nil {
			helper.WriteError(w, err)
			return
		}

		project, err := h.projectSvc.GetProject(r.Context(), projectID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		var updatedMember *dto.MemberResponse
		for _, m := range project.Members {
			if m.UserID == userID {
				updatedMember = m
				break
			}
		}

		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{"member": updatedMember})
	}
}

func (h *ProjectMemberHandler) RemoveMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		userID, err := helper.IDParam(r, "userId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.RemoveMember(r.Context(), projectID, userID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{"success": true})
	}
}
