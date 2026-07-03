package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type BoardServiceInterface interface {
	CreateBoard(ctx context.Context, projectID int64, req dto.CreateBoardRequest) (*model.Board, error)
	GetBoard(ctx context.Context, boardID int64) (*dto.BoardResponse, error)
	UpdateBoard(ctx context.Context, boardID int64, req dto.UpdateBoardRequest) (*model.Board, error)
	DeleteBoard(ctx context.Context, boardID int64) error
	GetBoardArchive(ctx context.Context, boardID int64) ([]model.Card, error)
}

type BoardService struct {
	repo        repository.BoardRepositoryInterface
	columnRepo  repository.ColumnRepositoryInterface
	cardRepo    repository.CardRepositoryInterface
	labelRepo   repository.LabelRepositoryInterface
	userRepo    repository.UserRepositoryInterface
	subtaskRepo repository.SubtaskRepositoryInterface
	commentRepo repository.CommentRepositoryInterface
	permSvc     *PermissionService
}

func NewBoardService(
	repo repository.BoardRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	cardRepo repository.CardRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	subtaskRepo repository.SubtaskRepositoryInterface,
	commentRepo repository.CommentRepositoryInterface,
	permSvc *PermissionService,
) *BoardService {
	return &BoardService{
		repo:        repo,
		columnRepo:  columnRepo,
		cardRepo:    cardRepo,
		labelRepo:   labelRepo,
		userRepo:    userRepo,
		subtaskRepo: subtaskRepo,
		commentRepo: commentRepo,
		permSvc:     permSvc,
	}
}

func (s *BoardService) CreateBoard(ctx context.Context, projectID int64, req dto.CreateBoardRequest) (*model.Board, error) {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	b := &model.Board{
		Title: req.Title,
	}
	if req.Position != nil {
		b.Position = *req.Position
	}
	return s.repo.CreateBoard(ctx, projectID, b)
}

func (s *BoardService) GetBoard(ctx context.Context, boardID int64) (*dto.BoardResponse, error) {
	b, err := s.repo.GetBoard(ctx, boardID)
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

			// Labels
			for _, lID := range card.LabelIDs {
				if l, ok := labelMap[lID]; ok {
					cardResp.Labels = append(cardResp.Labels, l)
				}
			}

			// Subtasks
			subtasks, _ := s.subtaskRepo.GetSubtasks(ctx, card.ID)
			cardResp.ChecklistTotal = len(subtasks)
			for _, st := range subtasks {
				if st.Status == "done" || st.Status == "DONE" {
					cardResp.ChecklistDone++
				}
			}

			// Comments
			comments, _ := s.commentRepo.GetComments(ctx, card.ID)
			cardResp.CommentsCount = len(comments)

			// Collect Assignees
			for _, uid := range card.AssigneeIDs {
				allUserIDs = append(allUserIDs, uid)
			}

			colResp.Cards = append(colResp.Cards, cardResp)
			allCards = append(allCards, cardResp)
		}
		boardResp.Columns = append(boardResp.Columns, colResp)
	}

	// Resolve Users
	if len(allUserIDs) > 0 {
		users, _ := s.userRepo.GetUsersByIDs(ctx, allUserIDs)
		userMap := make(map[int64]model.User)
		for _, u := range users {
			userMap[u.ID] = u
		}

		for _, cardResp := range allCards {
			for _, uid := range cardResp.AssigneeIDs {
				if u, ok := userMap[uid]; ok {
					name := u.Firstname
					if u.Lastname != "" {
						name += " " + u.Lastname
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

func (s *BoardService) UpdateBoard(ctx context.Context, boardID int64, req dto.UpdateBoardRequest) (*model.Board, error) {
	b, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleEditor); err != nil {
		return nil, err
	}

	if req.Title != nil {
		b.Title = *req.Title
	}
	if req.Position != nil {
		b.Position = *req.Position
	}
	return s.repo.UpdateBoard(ctx, b)
}

func (s *BoardService) DeleteBoard(ctx context.Context, boardID int64) error {
	b, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleEditor); err != nil {
		return err
	}
	
	hasColumns, err := s.repo.HasColumnsByBoard(ctx, boardID)
	if err == nil && hasColumns {
		return apperr.New(apperr.CodeValidation, "cannot delete board with columns")
	}

	return s.repo.DeleteBoard(ctx, boardID)
}

func (s *BoardService) GetBoardArchive(ctx context.Context, boardID int64) ([]model.Card, error) {
	b, err := s.repo.GetBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, b.KanbanProjectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetBoardArchive(ctx, boardID)
}
