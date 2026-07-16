package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
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
	cfg            *config.Config
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
	cfg *config.Config,
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
		cfg:            cfg,
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
	columns := normalizeBoardColumns(req.Columns)
	modelColumns := make([]model.Column, 0, len(columns))
	for i, column := range columns {
		modelColumns = append(modelColumns, model.Column{
			Title:       column.Title,
			HeaderColor: boardColumnColor(column.HeaderColor, i),
			Position:    float64(i + 1),
		})
	}

	return s.repo.CreateBoardWithColumns(ctx, projectID, b, modelColumns)
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

	// For enriching columnTitle on cards
	columnTitleByID := make(map[int64]string)
	for _, col := range columns {
		columnTitleByID[col.ID] = col.Title
	}

	// Получить все карточки доски одним запросом
	cards, err := s.cardRepo.GetCardsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	if len(cards) == 0 {
		// Пустая доска — собираем колонки без карточек
		for _, col := range columns {
			colResp := dto.MapColumnResponse(&col)
			colResp.Cards = []*dto.CardResponse{}
			boardResp.Columns = append(boardResp.Columns, colResp)
		}
		return boardResp, nil
	}

	// Собрать ID всех карточек
	cardIDs := make([]int64, len(cards))
	for i, card := range cards {
		cardIDs[i] = card.ID
	}

	// Bulk-запросы по всем card IDs
	assigneesByCard, err := s.cardRepo.GetAssigneesByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	labelsByCard, err := s.cardRepo.GetLabelIDsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	checklistsByCard, err := s.subtaskRepo.GetChecklistCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	commentCountsByCard, err := s.commentRepo.GetCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	chatCountsByCard, err := s.attachmentRepo.GetChatCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	// Проставить assignees/labels на карточки из bulk-результатов
	for i := range cards {
		cards[i].AssigneeIDs = assigneesByCard[cards[i].ID]
		cards[i].LabelIDs = labelsByCard[cards[i].ID]
	}

	// Сгруппировать карточки по columnID, сохранив порядок из запроса
	cardsByColumn := make(map[int64][]*dto.CardResponse)
	var allUserIDs []int64
	var allCards []*dto.CardResponse

	for i := range cards {
		card := &cards[i]
		cardResp := dto.MapCardResponse(card)

		// Проставить labels из labelMap
		for _, lID := range card.LabelIDs {
			if l, ok := labelMap[lID]; ok {
				cardResp.Labels = append(cardResp.Labels, l)
			}
		}

		// Set column title (useful for card components)
		if title, ok := columnTitleByID[card.ColumnID]; ok {
			cardResp.ColumnTitle = title
		}

		// Проставить checklist counts
		if checklist, ok := checklistsByCard[card.ID]; ok {
			cardResp.ChecklistTotal = checklist.Total
			cardResp.ChecklistDone = checklist.Done
		}

		// Проставить comments count (comments + chat attachments)
		cardResp.CommentsCount = commentCountsByCard[card.ID] + chatCountsByCard[card.ID]

		// Собрать user IDs для bulk-загрузки (assignees + creators)
		for _, uid := range card.AssigneeIDs {
			allUserIDs = append(allUserIDs, uid)
		}
		if card.CreatedByID != nil {
			allUserIDs = append(allUserIDs, *card.CreatedByID)
		}
		if card.CompletedByID != nil {
			allUserIDs = append(allUserIDs, *card.CompletedByID)
		}

		cardsByColumn[card.ColumnID] = append(cardsByColumn[card.ColumnID], cardResp)
		allCards = append(allCards, cardResp)
	}

	// Bulk-загрузка пользователей assignees
	if len(allUserIDs) > 0 {
		users, err := s.userRepo.GetUsersByIDs(ctx, allUserIDs)
		if err != nil {
			return nil, err
		}
		userMap := make(map[int64]model.User)
		for _, u := range users {
			userMap[u.ID] = u
		}

		for _, cardResp := range allCards {
			for _, uid := range cardResp.AssigneeIDs {
				if u, ok := userMap[uid]; ok {
					name := dto.UserDisplayName(u)
					if name == "" {
						name = strings.TrimSpace(u.Firstname)
					}
					cardResp.Assignees = append(cardResp.Assignees, &dto.CardAssigneeResponse{
						ID:        u.ID,
						Name:      name,
						AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
					})
				}
			}

			// Enrich createdBy / completedBy + boardId for cards in the board
			if cardResp.CreatedByID != nil {
				if u, ok := userMap[*cardResp.CreatedByID]; ok {
					cardResp.CreatedBy = &dto.CardUserResponse{
						ID:        u.ID,
						Firstname: u.Firstname,
						Lastname:  u.Lastname,
						AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
					}
				}
			}
			if cardResp.CompletedByID != nil {
				if u, ok := userMap[*cardResp.CompletedByID]; ok {
					cardResp.CompletedBy = &dto.CardUserResponse{
						ID:        u.ID,
						Firstname: u.Firstname,
						Lastname:  u.Lastname,
						AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
					}
				}
			}
		}

	}

	// Set boardId for all cards (known from context) - always, even if no users
	for _, cardResp := range allCards {
		cardResp.BoardID = boardID
	}

	// Собрать колонки с карточками
	for _, col := range columns {
		colResp := dto.MapColumnResponse(&col)
		colResp.Cards = cardsByColumn[col.ID]
		if colResp.Cards == nil {
			colResp.Cards = []*dto.CardResponse{}
		}
		boardResp.Columns = append(boardResp.Columns, colResp)
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
				return nil, apperr.New(apperr.CodeBoardTitleTooLong, "board title too long")
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
		return nil, apperr.New(apperr.CodeBoardHasCards, "cannot delete board with active cards")
	}

	nextBoardID, err := s.repo.NextBoardID(ctx, projectID, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeleteBoard(ctx, boardID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, apperr.New(apperr.CodeBoardHasCards, "cannot delete board with cards")
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
	return dto.MapBoardArchiveResponse(s.cfg, archive), nil
}

func (s *BoardService) resolveBoard(ctx context.Context, projectID int64, boardID int64) (*model.Board, error) {
	board, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeBoardNotFound)
	}
	if board.KanbanProjectID != projectID {
		return nil, apperr.New(apperr.CodeBoardNotFound, "board not found")
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
		return "", apperr.New(apperr.CodeBoardTitleRequired, "board title required")
	}
	if utf8.RuneCountInString(title) > maxBoardTitleLength {
		return "", apperr.New(apperr.CodeBoardTitleTooLong, "board title too long")
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
