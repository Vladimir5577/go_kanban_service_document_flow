package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/service"
	"go_kanban_service/internal/validator"
)

type ProjectHandler struct {
	service service.ProjectServiceInterface
}

func NewProjectHandler(s service.ProjectServiceInterface) *ProjectHandler {
	return &ProjectHandler{service: s}
}

func (h *ProjectHandler) ListProjects() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		status := query.Get("status")
		if status == "" {
			status = "active"
		}
		switch status {
		case "active", "deleted", "all":
		default:
			helper.WriteError(w, apperr.New(apperr.CodeValidation, "invalid status filter"))
			return
		}

		pageSize := parsePositiveInt(query.Get("page_size"), 10)
		if pageSize > 100 {
			pageSize = 100
		}

		orderBy := query.Get("order_by")
		if orderBy == "" {
			orderBy = "created_at"
		}
		switch orderBy {
		case "name", "created_at", "members_count", "boards_count", "tasks_count":
		default:
			helper.WriteError(w, apperr.New(apperr.CodeValidation, "invalid order_by"))
			return
		}

		order := strings.ToUpper(query.Get("order"))
		if order == "" {
			order = "DESC"
		}
		if order != "ASC" && order != "DESC" {
			helper.WriteError(w, apperr.New(apperr.CodeValidation, "invalid order"))
			return
		}

		res, err := h.service.ListProjects(r.Context(), model.ProjectListFilters{
			Search:  strings.TrimSpace(query.Get("search")),
			Status:  status,
			OrderBy: orderBy,
			Order:   order,
			Page:    parsePositiveInt(query.Get("page"), 1),
			Limit:   pageSize,
		})
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *ProjectHandler) GetMyProjects() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := h.service.GetNavProjectsForUser(r.Context())
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, projects)
	}
}

func (h *ProjectHandler) CreateProject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req dto.CreateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Name", Tag: "required"}: apperr.CodeProjectNameRequired,
				{Field: "Name", Tag: "min"}:      apperr.CodeProjectNameTooShort,
				{Field: "Name", Tag: "max"}:      apperr.CodeProjectNameTooLong,
			}))
			return
		}

		created, err := h.service.CreateProject(r.Context(), req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapProjectResponse(created))
	}
}

func (h *ProjectHandler) GetProject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		project, err := h.service.GetProject(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, project)
	}
}

func (h *ProjectHandler) UpdateProject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Name", Tag: "min"}: apperr.CodeProjectNameTooShort,
				{Field: "Name", Tag: "max"}: apperr.CodeProjectNameTooLong,
			}))
			return
		}

		updated, err := h.service.UpdateProject(r.Context(), id, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapProjectResponse(updated))
	}
}

func (h *ProjectHandler) MoveProject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.MoveProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, nil))
			return
		}

		moved, err := h.service.MoveProject(r.Context(), id, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, moved)
	}
}

func (h *ProjectHandler) DeleteProject() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteProject(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
