package handler

import (
	"net/http"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type ProjectFolderHandler struct {
	service service.ProjectFolderServiceInterface
}

func NewProjectFolderHandler(s service.ProjectFolderServiceInterface) *ProjectFolderHandler {
	return &ProjectFolderHandler{service: s}
}

func (h *ProjectFolderHandler) GetProjectFolders() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := h.service.GetProjectFolders(r.Context())
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapProjectFoldersResponse(res))
	}
}
