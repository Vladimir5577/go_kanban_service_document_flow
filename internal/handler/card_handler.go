package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/repository"
	"go_kanban_service/internal/service"
	"go_kanban_service/internal/validator"
)

type CardHandler struct {
	service service.CardServiceInterface
}

func NewCardHandler(s service.CardServiceInterface) *CardHandler {
	return &CardHandler{service: s}
}

func (h *CardHandler) CreateCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req dto.CreateCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Title", Tag: "required"}: apperr.CodeColumnIDAndTitleRequired,
			}))
			return
		}

		created, err := h.service.CreateCard(r.Context(), req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, created)
	}
}

func (h *CardHandler) DuplicateCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		var req dto.DuplicateCardRequest
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				helper.WriteError(w, invalidJSONError())
				return
			}
		}
		created, err := h.service.DuplicateCard(r.Context(), id, req.ColumnID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, created)
	}
}

func (h *CardHandler) GetCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *CardHandler) GetCardStandalone() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetCardStandalone(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *CardHandler) ListTasks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := parseTaskListQuery(r)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		res, err := h.service.ListTasks(r.Context(), f)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *CardHandler) ListTaskCollaborants() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page := parsePositiveInt(q.Get("page"), 1)
		pageSize := parsePositiveInt(q.Get("page_size"), 20)
		if pageSize > 100 {
			pageSize = 100
		}
		res, err := h.service.ListTaskCollaborants(r.Context(), repository.TaskCollaborantsParams{
			NameQuery: likeQuery(q.Get("search")),
			Limit:     int32(pageSize),
			Offset:    pageOffset(page, pageSize),
		})
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *CardHandler) UpdateCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, nil))
			return
		}

		resp, err := h.service.UpdateCard(r.Context(), id, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, resp)
	}
}

func (h *CardHandler) DeleteCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteCard(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}

func (h *CardHandler) UpdateAssignees() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var payload struct {
			UserIDs []int64 `json:"user_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		assignees, err := h.service.UpdateAssignees(r.Context(), id, payload.UserIDs)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"assignees": assignees,
		})
	}
}

func (h *CardHandler) MoveCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var payload struct {
			ColumnID *int64  `json:"column_id"`
			Position float64 `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		move, err := h.service.MoveCard(r.Context(), id, payload.ColumnID, payload.Position)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapMoveCardResponse(move))
	}
}

func (h *CardHandler) ArchiveCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		archived, err := h.service.ArchiveCard(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"id":         id,
			"isArchived": archived,
		})
	}
}

func (h *CardHandler) CompleteCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		resp, err := h.service.CompleteCard(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, resp)
	}
}

func parseTaskListQuery(r *http.Request) (repository.TaskListParams, error) {
	q := r.URL.Query()
	invalid := apperr.New(apperr.CodeValidation, "invalid task list filter")

	priority := strings.TrimSpace(q.Get("priority"))
	switch priority {
	case "", "high", "medium", "low", "none":
	default:
		return repository.TaskListParams{}, invalid
	}

	kind := strings.TrimSpace(q.Get("kind"))
	switch kind {
	case "", "task", "subtask":
	default:
		return repository.TaskListParams{}, invalid
	}

	completed, err := parseTriBool(q.Get("completed"))
	if err != nil {
		return repository.TaskListParams{}, err
	}
	archived, err := parseTriBool(q.Get("archived"))
	if err != nil {
		return repository.TaskListParams{}, err
	}

	sort := strings.TrimSpace(q.Get("sort"))
	if sort == "" {
		sort = "priority"
	}
	switch sort {
	case "title", "createdAt", "completedAt", "priority":
	default:
		return repository.TaskListParams{}, invalid
	}
	order := strings.ToLower(strings.TrimSpace(q.Get("order")))
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		return repository.TaskListParams{}, invalid
	}

	page := parsePositiveInt(q.Get("page"), 1)
	pageSize := parsePositiveInt(q.Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	dueFrom, dueTo, err := parseDayRange(q.Get("due_from"), q.Get("due_to"))
	if err != nil {
		return repository.TaskListParams{}, err
	}
	createdFrom, createdTo, err := parseDayRange(q.Get("created_from"), q.Get("created_to"))
	if err != nil {
		return repository.TaskListParams{}, err
	}
	completedFrom, completedTo, err := parseDayRange(q.Get("completed_from"), q.Get("completed_to"))
	if err != nil {
		return repository.TaskListParams{}, err
	}
	archivedFrom, archivedTo, err := parseDayRange(q.Get("archived_from"), q.Get("archived_to"))
	if err != nil {
		return repository.TaskListParams{}, err
	}

	return repository.TaskListParams{
		TitleQuery:    likeQuery(q.Get("search")),
		ProjectID:     parseOptionalID(q.Get("project_id")),
		Priority:      priority,
		AuthorID:      parseUserFilter(q.Get("author_id")),
		AssigneeID:    parseUserFilter(q.Get("assignee_id")),
		Completed:     completed,
		Archived:      archived,
		Kind:          kind,
		DueFrom:       dueFrom,
		DueTo:         dueTo,
		CreatedFrom:   createdFrom,
		CreatedTo:     createdTo,
		CompletedFrom: completedFrom,
		CompletedTo:   completedTo,
		ArchivedFrom:  archivedFrom,
		ArchivedTo:    archivedTo,
		Sort:          sort,
		SortDesc:      order == "desc",
		Limit:         int32(pageSize),
		Offset:        pageOffset(page, pageSize),
	}, nil
}

// pageOffset считает смещение с защитой от переполнения: page сверху не
// ограничен, и (page-1)*pageSize при большом page уезжал в минус после
// приведения к int32 — база отвечала ошибкой на отрицательный OFFSET.
func pageOffset(page, pageSize int) int32 {
	offset := int64(page-1) * int64(pageSize)
	if offset < 0 {
		return 0
	}
	if offset > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(offset)
}

func parseTriBool(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return "", nil
	case "true", "1":
		return "true", nil
	case "false", "0":
		return "false", nil
	default:
		return "", apperr.New(apperr.CodeValidation, "invalid task list filter")
	}
}

func parseOptionalID(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "all" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

func parseUserFilter(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "all") {
		return 0
	}
	if strings.EqualFold(raw, "none") {
		return -1
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func parseDayRange(fromRaw, toRaw string) (from, to *time.Time, err error) {
	if fromRaw != "" {
		t, parseErr := time.Parse("2006-01-02", fromRaw)
		if parseErr != nil {
			return nil, nil, apperr.New(apperr.CodeValidation, "invalid task list filter")
		}
		from = &t
	}
	if toRaw != "" {
		t, parseErr := time.Parse("2006-01-02", toRaw)
		if parseErr != nil {
			return nil, nil, apperr.New(apperr.CodeValidation, "invalid task list filter")
		}
		end := t.Add(24 * time.Hour)
		to = &end
	}
	return from, to, nil
}

func likeQuery(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
