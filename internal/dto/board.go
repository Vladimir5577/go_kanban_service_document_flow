package dto

import (
	"encoding/json"
	"strconv"
	"time"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/model"
)

type CreateBoardColumnRequest struct {
	Title       string  `json:"title"`
	HeaderColor *string `json:"headerColor,omitempty"`
}

type CreateBoardRequest struct {
	Title    string                     `json:"title" validate:"required"`
	Position *float64                   `json:"position,omitempty"`
	Columns  []CreateBoardColumnRequest `json:"columns,omitempty"`
}

func (r *CreateBoardRequest) UnmarshalJSON(data []byte) error {
	var payload struct {
		Title    string          `json:"title"`
		Position *float64        `json:"position"`
		Columns  json.RawMessage `json:"columns"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.Title = payload.Title
	r.Position = payload.Position
	r.Columns = nil

	if len(payload.Columns) == 0 || string(payload.Columns) == "null" {
		return nil
	}

	var rawColumns []json.RawMessage
	if err := json.Unmarshal(payload.Columns, &rawColumns); err != nil {
		return nil
	}

	for _, raw := range rawColumns {
		var title string
		if err := json.Unmarshal(raw, &title); err == nil {
			r.Columns = append(r.Columns, CreateBoardColumnRequest{Title: title})
			continue
		}

		var column struct {
			Title             string  `json:"title"`
			HeaderColor       *string `json:"headerColor"`
			HeaderColorLegacy *string `json:"header_color"`
		}
		if err := json.Unmarshal(raw, &column); err != nil {
			continue
		}
		headerColor := column.HeaderColor
		if headerColor == nil {
			headerColor = column.HeaderColorLegacy
		}
		r.Columns = append(r.Columns, CreateBoardColumnRequest{
			Title:       column.Title,
			HeaderColor: headerColor,
		})
	}
	return nil
}

type UpdateBoardRequest struct {
	Title    *string  `json:"title,omitempty" validate:"omitempty"`
	Position *float64 `json:"position,omitempty"`
}

type BoardResponse struct {
	ID              int64             `json:"id"`
	Title           string            `json:"title"`
	Position        float64           `json:"position"`
	KanbanProjectID int64             `json:"kanbanProjectId"`
	CreatedByID     int64             `json:"createdById"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	Columns         []*ColumnResponse `json:"columns"`
}

func MapBoardResponse(b *model.Board) *BoardResponse {
	if b == nil {
		return nil
	}
	return &BoardResponse{
		ID:              b.ID,
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: b.KanbanProjectID,
		CreatedByID:     b.CreatedByID,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
		Columns:         make([]*ColumnResponse, 0),
	}
}

func MapBoardsResponse(boards []model.Board) []*BoardResponse {
	resp := make([]*BoardResponse, 0, len(boards))
	for i := range boards {
		resp = append(resp, MapBoardResponse(&boards[i]))
	}
	return resp
}

type DeleteBoardResponse struct {
	Success     bool   `json:"success"`
	NextBoardID *int64 `json:"nextBoardId"`
}

type BoardArchivePaginationResponse struct {
	CurrentPage int   `json:"currentPage"`
	TotalPages  int   `json:"totalPages"`
	Total       int64 `json:"total"`
	Limit       int   `json:"limit"`
}

type ArchivedCardResponse struct {
	ID          int64                 `json:"id"`
	Title       string                `json:"title"`
	Description *string               `json:"description"`
	ColumnTitle string                `json:"columnTitle"`
	BorderColor *string               `json:"borderColor"`
	ArchivedAt  *time.Time            `json:"archivedAt"`
	ArchivedBy  *CardAssigneeResponse `json:"archivedBy"`
}

type BoardArchiveResponse struct {
	Cards         []*ArchivedCardResponse        `json:"cards"`
	Pagination    BoardArchivePaginationResponse `json:"pagination"`
	ArchivedCount int64                          `json:"archivedCount"`
}

func MapBoardArchiveResponse(cfg *config.Config, page *model.BoardArchivePage) *BoardArchiveResponse {
	if page == nil {
		return nil
	}

	totalPages := 0
	if page.Limit > 0 && page.Total > 0 {
		totalPages = int((page.Total + int64(page.Limit) - 1) / int64(page.Limit))
	}

	resp := &BoardArchiveResponse{
		Cards: make([]*ArchivedCardResponse, 0, len(page.Cards)),
		Pagination: BoardArchivePaginationResponse{
			CurrentPage: page.Page,
			TotalPages:  totalPages,
			Total:       page.Total,
			Limit:       page.Limit,
		},
		ArchivedCount: page.ArchivedCount,
	}

	for i := range page.Cards {
		resp.Cards = append(resp.Cards, MapArchivedCardResponse(cfg, &page.Cards[i]))
	}
	return resp
}

func MapArchivedCardResponse(cfg *config.Config, card *model.ArchivedCard) *ArchivedCardResponse {
	if card == nil {
		return nil
	}
	return &ArchivedCardResponse{
		ID:          card.ID,
		Title:       card.Title,
		Description: card.Description,
		ColumnTitle: card.ColumnTitle,
		BorderColor: card.BorderColor,
		ArchivedAt:  card.ArchivedAt,
		ArchivedBy:  mapArchivedByUser(cfg, card.ArchivedBy),
	}
}

func mapArchivedByUser(cfg *config.Config, user *model.User) *CardAssigneeResponse {
	if user == nil {
		return nil
	}

	name := UserDisplayName(*user)
	if name == "" {
		name = strconv.FormatInt(user.ID, 10)
	}
	return &CardAssigneeResponse{
		ID:        user.ID,
		Name:      name,
		AvatarUrl: UserAvatarURL(cfg, user.AvatarName, AvatarSizeThumbnail),
	}
}
