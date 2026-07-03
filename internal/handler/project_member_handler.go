package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type ProjectMemberHandler struct {
	service service.ProjectMemberServiceInterface
}

func NewProjectMemberHandler(s service.ProjectMemberServiceInterface) *ProjectMemberHandler {
	return &ProjectMemberHandler{service: s}
}

func (h *ProjectMemberHandler) ReplaceMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var reqs []dto.AddProjectMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}

		if err := h.service.ReplaceMembers(r.Context(), projectID, reqs); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]string{"status": "replaced"})
	}
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
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}

		if err := h.service.UpdateMemberRole(r.Context(), projectID, userID, req); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
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
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
