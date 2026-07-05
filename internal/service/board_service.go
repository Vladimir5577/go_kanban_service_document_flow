package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const maxBoardTitleLength = 200

var defaultBoardColumns = []dto.CreateBoardColumnRequest{
	{Title: "К выполнению", HeaderColor: boardColorPtr("bg-success")},
	{Title: "В работе", HeaderColor: boardColorPtr("bg-primary")},
	{Title: "Сделаны", HeaderColor: boardColorPtr("bg-warning")},
	{Title: "Проверены", HeaderColor: boardColorPtr("bg-danger")},
}

var defaultBoardColumnColors = []string{
	"bg-success",
	"bg-primary",
	"bg-warning",
	"bg-danger",
}

type BoardServiceInterface interface {
	CreateBoard(ctx context.Context, projectID int64, req dto.CreateBoardRequest) (*model.Board, error)
	GetBoard(ctx context.Context, projectID int64, boardID int64) (*dto.BoardResponse, error)
	UpdateBoard(ctx context.Context, projectID int64, boardID int64, req dto.UpdateBoardRequest) (*model.Board, error)
	DeleteBoard(ctx context.Context, projectID int64, boardID int64) (*dto.DeleteBoardResponse, error)
	GetBoardArchive(ctx context.Context, projectID int64, boardID int64, filters model.BoardArchiveFilters) (*dto.BoardArchiveResponse, error)
}

type BoardService struct {
	repo           repository.BoardRepositoryInterface
	columnRepo     repository.ColumnRepositoryInterface
	cardRepo       repository.CardRepositoryInterface
	labelRepo      repository.LabelRepositoryInterface
	userRepo       repository.UserRepositoryInterface
	subtaskRepo    repository.SubtaskRepositoryInterface
	commentRepo    repository.CommentRepositoryInterface
	attachmentRepo repository.AttachmentRepositoryInterface
	permSvc        *PermissionService
}

func NewBoardService(
	repo repository.BoardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	subtaskRepo repository.SubtaskRepositoryInterface,
	commentRepo repository.CommentRepositoryInterface,
	attachmentRepo repository.AttachmentRepositoryInterface,
	permSvc *PermissionService,
) *BoardService {
	return &BoardService{
		repo:           repo,
		columnRepo:     columnRepo,
		cardRepo:       cardRepo,
		labelRepo:      labelRepo,
		userRepo:       userRepo,
		subtaskRepo:    subtaskRepo,
		commentRepo:    commentRepo,
		attachmentRepo: attachmentRepo,
		permSvc:        permSvc,
	}
}

func (s *BoardService) CreateBoard(ctx context.Context, projectID int64, req dto.CreateBoardRequest) (*model.Board, error) {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return nil, err
	}

	title, err := normalizeBoardTitle(req.Title)
	if err != nil {
		return nil, err
	}
	authorID := currentUserID(ctx)
	if authorID == nil {
		return nil, apperr.ErrUnauthorized
	}

	position, err := s.nextBoardPosition(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if req.Position != nil {
		position = *req.Position
	}

	b := &model.Board{
		Title:       title,
		Position:    position,
		CreatedByID: *authorID,
	}
	created, err := s.repo.CreateBoard(ctx, projectID, b)
	if err != nil {
		return nil, err
	}

	columns := normalizeBoardColumns(req.Columns)
	for i, column := range columns {
		color := boardColumnColor(column.HeaderColor, i)
		if _, err := s.columnRepo.CreateColumn(ctx, created.ID, &model.Column{
			Title:       column.Title,
			HeaderColor: color,
			Position:    float64(i + 1),
			BoardID:     created.ID,
		}); err != nil {
			return nil, err
		}
	}

	return created, nil
}

func (s *BoardService) GetBoard(ctx context.Context, projectID int64, boardID int64) (*dto.BoardResponse, error) {
	b, err := s.resolveBoard(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleViewer); err != nil {
		return nil, err
	}

	boardResp := dto.MapBoardResponse(b)

	columns, err := s.columnRepo.GetColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	labels, err := s.labelRepo.GetLabels(ctx, boardID)
	if err != nil {
		return nil, err
	}
	labelMap := make(map[int64]*dto.LabelResponse)
	for i := range labels {
		l := dto.MapLabelResponse(&labels[i])
		labelMap[l.ID] = l
	}

	var allUserIDs []int64
	var allCards []*dto.CardResponse

	for _, col := range columns {
		colResp := dto.MapColumnResponse(&col)

		cards, _ := s.cardRepo.GetCardsByColumn(ctx, col.ID)
		for _, card := range cards {
			cardResp := dto.MapCardResponse(&card)

			for _, lID := range card.LabelIDs {
				if l, ok := labelMap[lID]; ok {
					cardResp.Labels = append(cardResp.Labels, l)
				}
			}

			subtasks, _ := s.subtaskRepo.GetSubtasks(ctx, card.ID)
			cardResp.ChecklistTotal = len(subtasks)
			for _, st := range subtasks {
				if st.Status == "done" || st.Status == "DONE" {
					cardResp.ChecklistDone++
				}
			}

			comments, _ := s.commentRepo.GetComments(ctx, card.ID)
			chatAttachments, _ := s.attachmentRepo.GetAttachmentsByCard(ctx, card.ID, "chat")
			cardResp.CommentsCount = len(comments) + len(chatAttachments)

			for _, uid := range card.AssigneeIDs {
				allUserIDs = append(allUserIDs, uid)
			}

			colResp.Cards = append(colResp.Cards, cardResp)
			allCards = append(allCards, cardResp)
		}
		boardResp.Columns = append(boardResp.Columns, colResp)
	}

	if len(allUserIDs) > 0 {
		users, _ := s.userRepo.GetUsersByIDs(ctx, allUserIDs)
		userMap := make(map[int64]model.User)
		for _, u := range users {
			userMap[u.ID] = u
		}

		for _, cardResp := range allCards {
			for _, uid := range cardResp.AssigneeIDs {
				if u, ok := userMap[uid]; ok {
					name := strings.TrimSpace(u.Lastname + " " + u.Firstname)
					if name == "" {
						name = strings.TrimSpace(u.Firstname)
					}
					cardResp.Assignees = append(cardResp.Assignees, &dto.CardAssigneeResponse{
						ID:        u.ID,
						Name:      name,
						AvatarUrl: u.AvatarName,
					})
				}
			}
		}
	}

	return boardResp, nil
}

func (s *BoardService) UpdateBoard(ctx context.Context, projectID int64, boardID int64, req dto.UpdateBoardRequest) (*model.Board, error) {
	b, err := s.resolveBoard(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleAdmin); err != nil {
		return nil, err
	}

	changed := false
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title != "" {
			if utf8.RuneCountInString(title) > maxBoardTitleLength {
				return nil, apperr.New(apperr.CodeValidation, "board title too long")
			}
			b.Title = title
			changed = true
		}
	}
	if req.Position != nil {
		b.Position = *req.Position
		changed = true
	}
	if !changed {
		return b, nil
	}
	return s.repo.UpdateBoard(ctx, b)
}

func (s *BoardService) DeleteBoard(ctx context.Context, projectID int64, boardID int64) (*dto.DeleteBoardResponse, error) {
	b, err := s.resolveBoard(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleAdmin); err != nil {
		return nil, err
	}

	hasActiveCards, err := s.repo.HasActiveCardsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if hasActiveCards {
		return nil, apperr.New(apperr.CodeConflict, "cannot delete board with active cards")
	}

	nextBoardID, err := s.repo.NextBoardID(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeleteBoard(ctx, boardID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, apperr.New(apperr.CodeValidation, "Нельзя удалить доску, пока на ней есть задачи (включая архивные).")
		}
		return nil, err
	}

	return &dto.DeleteBoardResponse{Success: true, NextBoardID: nextBoardID}, nil
}

func (s *BoardService) GetBoardArchive(ctx context.Context, projectID int64, boardID int64, filters model.BoardArchiveFilters) (*dto.BoardArchiveResponse, error) {
	b, err := s.resolveBoard(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleViewer); err != nil {
		return nil, err
	}
	archive, err := s.repo.GetBoardArchive(ctx, boardID, filters)
	if err != nil {
		return nil, err
	}
	return dto.MapBoardArchiveResponse(archive), nil
}

func (s *BoardService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, mapNoRowsToNotFound(err)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.ErrNotFound
	}
	return board, nil
}

func (s *BoardService) nextBoardPosition(ctx context.Context, projectID int64) (float64, error) {
	boards, err := s.repo.GetBoardsByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	maxPosition := 0.0
	for _, board := range boards {
		maxPosition = math.Max(maxPosition, board.Position)
	}
	return maxPosition + 1.0, nil
}

func normalizeBoardTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", apperr.New(apperr.CodeValidation, "board title required")
	}
	if utf8.RuneCountInString(title) > maxBoardTitleLength {
		return "", apperr.New(apperr.CodeValidation, "board title too long")
	}
	return title, nil
}

func normalizeBoardColumns(columns []dto.CreateBoardColumnRequest) []dto.CreateBoardColumnRequest {
	normalized := make([]dto.CreateBoardColumnRequest, 0, len(columns))
	for _, column := range columns {
		title := strings.TrimSpace(column.Title)
		if title == "" {
			continue
		}
		var headerColor *string
		if column.HeaderColor != nil {
			if color, ok := normalizeColumnColorForUpdate(*column.HeaderColor); ok {
				headerColor = &color
			}
		}
		normalized = append(normalized, dto.CreateBoardColumnRequest{Title: title, HeaderColor: headerColor})
	}
	if len(normalized) == 0 {
		return defaultBoardColumns
	}
	return normalized
}

func boardColumnColor(color *string, index int) string {
	if color != nil {
		return *color
	}
	if len(defaultBoardColumnColors) == 0 {
		return defaultColumnColor
	}
	return defaultBoardColumnColors[index%len(defaultBoardColumnColors)]
}

func boardColorPtr(color string) *string {
	return &color
}
