package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const defaultProjectBoardTitle = "Главная доска"

type ProjectServiceInterface interface {
	GetAllProjects(ctx context.Context) ([]model.Project, error)
	CreateProject(ctx context.Context, req dto.CreateProjectRequest) (*model.Project, error)
	GetProject(ctx context.Context, id int64) (*dto.ProjectResponse, error)
	UpdateProject(ctx context.Context, id int64, req dto.UpdateProjectRequest) (*model.Project, error)
	MoveProject(ctx context.Context, id int64, req dto.MoveProjectRequest) (*dto.MoveProjectResponse, error)
	DeleteProject(ctx context.Context, id int64) error
	GetNavProjectsForUser(ctx context.Context) ([]*dto.NavProjectResponse, error)
}

type ProjectService struct {
	repo       repository.ProjectRepositoryInterface
	boardRepo  repository.BoardRepositoryInterface
	memberRepo repository.ProjectMemberRepositoryInterface
	folderRepo repository.ProjectFolderRepositoryInterface
	userRepo   repository.UserRepositoryInterface
	permSvc    *PermissionService
	cfg        *config.Config
}

func NewProjectService(
	repo repository.ProjectRepositoryInterface,
	boardRepo repository.BoardRepositoryInterface,
	memberRepo repository.ProjectMemberRepositoryInterface,
	folderRepo repository.ProjectFolderRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	permSvc *PermissionService,
	cfg *config.Config,
) *ProjectService {
	return &ProjectService{
		repo:       repo,
		boardRepo:  boardRepo,
		memberRepo: memberRepo,
		folderRepo: folderRepo,
		userRepo:   userRepo,
		permSvc:    permSvc,
		cfg:        cfg,
	}
}

func (s *ProjectService) GetAllProjects(ctx context.Context) ([]model.Project, error) {
	return s.repo.GetAllProjects(ctx)
}

func (s *ProjectService) CreateProject(ctx context.Context, req dto.CreateProjectRequest) (*model.Project, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, accessDenied()
	}

	p := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     user.ID,
		CreatedByID: &user.ID,
	}
	created, err := s.repo.CreateProject(ctx, p)
	if err != nil {
		return nil, apperr.New(apperr.CodeProjectCreateFailed, "project create failed")
	}

	err = s.memberRepo.ReplaceMembers(ctx, created.ID, []model.ProjectUser{
		{
			KanbanProjectID: created.ID,
			UserID:          user.ID,
			Role:            string(RoleAdmin),
		},
	})
	if err != nil {
		return nil, apperr.New(apperr.CodeProjectCreateFailed, "project create failed")
	}

	board, err := s.createDefaultBoard(ctx, created.ID, user.ID)
	if err != nil {
		return nil, apperr.New(apperr.CodeProjectCreateFailed, "project create failed")
	}
	created.EntryBoardID = &board.ID
	return created, nil
}

func (s *ProjectService) createDefaultBoard(ctx context.Context, projectID int64, userID int64) (*model.Board, error) {
	columns := normalizeBoardColumns(defaultBoardColumns)
	modelColumns := make([]model.Column, 0, len(columns))
	for i, column := range columns {
		modelColumns = append(modelColumns, model.Column{
			Title:       column.Title,
			HeaderColor: boardColumnColor(column.HeaderColor, i),
			Position:    float64(i + 1),
		})
	}

	return s.boardRepo.CreateBoardWithColumns(ctx, projectID, &model.Board{
		Title:       defaultProjectBoardTitle,
		Position:    1,
		CreatedByID: userID,
	}, modelColumns)
}

func (s *ProjectService) GetProject(ctx context.Context, id int64) (*dto.ProjectResponse, error) {
	if err := s.permSvc.RequireRole(ctx, id, RoleViewer); err != nil {
		return nil, err
	}

	p, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}

	boards, err := s.boardRepo.GetBoardsByProject(ctx, id)
	if err != nil {
		return nil, err
	}

	members, err := s.memberRepo.GetMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	members = ensureOwnerMember(members, p.OwnerID, id)

	// Собрать всех пользователей, которых нужно загрузить
	var userIDs []int64
	userIDs = append(userIDs, p.OwnerID)
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	// Построить словарь для быстрого поиска
	userMap := make(map[int64]model.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	resp := dto.MapProjectResponse(p)

	// Владелец
	if owner, ok := userMap[p.OwnerID]; ok {
		resp.Owner = &dto.UserResponse{
			ID:         owner.ID,
			Login:      owner.Login,
			Lastname:   owner.Lastname,
			Firstname:  owner.Firstname,
			Patronymic: owner.Patronymic,
		}
	}

	// Доски
	resp.Boards = dto.MapBoardsResponse(boards)

	// Участники
	for _, m := range members {
		memResp := &dto.MemberResponse{
			UserID: m.UserID,
			Role:   m.Role,
			// TODO: RoleLabel
		}
		if u, ok := userMap[m.UserID]; ok {
			memResp.Login = u.Login
			memResp.Lastname = u.Lastname
			memResp.Firstname = u.Firstname
			memResp.Patronymic = u.Patronymic
			memResp.AvatarUrl = dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail)
			memResp.IsOwner = (u.ID == p.OwnerID)
		}
		resp.Members = append(resp.Members, memResp)
	}

	// Права текущего пользователя
	if currUser, ok := middleware.GetUser(ctx); ok {
		resp.IsOwner = (currUser.ID == p.OwnerID)
		for _, m := range members {
			if m.UserID == currUser.ID {
				resp.MemberRole = m.Role
				resp.IsProjectAdmin = (m.Role == string(RoleAdmin) || resp.IsOwner)
				break
			}
		}
	}

	return resp, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, id int64, req dto.UpdateProjectRequest) (*model.Project, error) {
	if err := s.permSvc.RequireRole(ctx, id, RoleEditor); err != nil {
		return nil, err
	}

	p, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeProjectNotFound)
	}
	if req.Name == nil && req.Description == nil {
		return nil, apperr.New(apperr.CodeUpdateFieldsRequired, "update fields required")
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, apperr.New(apperr.CodeProjectNameRequired, "project name required")
		}
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	return s.repo.UpdateProject(ctx, p)
}

func (s *ProjectService) MoveProject(ctx context.Context, id int64, req dto.MoveProjectRequest) (*dto.MoveProjectResponse, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, accessDenied()
	}
	if req.Position == nil {
		return nil, apperr.New(apperr.CodeInvalidJSON, "position required")
	}
	if req.FolderID != nil && *req.FolderID <= 0 {
		return nil, apperr.New(apperr.CodeFolderNotFound, "folder not found")
	}

	project, err := s.repo.GetProject(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeProjectNotFound)
	}

	if req.FolderID != nil {
		folder, err := s.folderRepo.GetProjectFolder(ctx, *req.FolderID)
		if err != nil {
			return nil, withNotFoundCode(err, apperr.CodeFolderNotFound)
		}
		if folder.UserID != user.ID {
			return nil, accessDenied()
		}
	}

	member, err := s.memberRepo.GetProjectMember(ctx, id, user.ID)
	if err != nil {
		if !errors.Is(err, apperr.ErrNotFound) {
			return nil, err
		}
		if project.OwnerID != user.ID {
			return nil, accessDenied()
		}
		if err := s.memberRepo.AddMember(ctx, id, model.ProjectUser{
			KanbanProjectID: id,
			UserID:          user.ID,
			Role:            string(RoleAdmin),
		}); err != nil {
			return nil, err
		}
	}

	member, err = s.memberRepo.UpdateProjectPlacement(ctx, id, user.ID, req.FolderID, *req.Position)
	if err != nil {
		return nil, err
	}

	rebalanced, err := s.memberRepo.RebalanceProjectPositions(ctx, user.ID, req.FolderID)
	if err != nil {
		return nil, err
	}
	var rebalancedProjects []*dto.NavProjectResponse
	if rebalanced {
		member, err = s.memberRepo.GetProjectMember(ctx, id, user.ID)
		if err != nil {
			return nil, err
		}
		navProjects, err := s.repo.GetNavProjectsForUser(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		rebalancedProjects = make([]*dto.NavProjectResponse, 0, len(navProjects))
		for _, p := range navProjects {
			rebalancedProjects = append(rebalancedProjects, dto.MapNavProjectResponse(p, user.ID))
		}
	}

	return &dto.MoveProjectResponse{
		ID:                 project.ID,
		FolderID:           member.FolderID,
		Position:           member.Position,
		RebalancedProjects: rebalancedProjects,
	}, nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, id int64) error {
	if err := s.permSvc.RequireRole(ctx, id, RoleAdmin); err != nil {
		return err
	}
	err := s.repo.DeleteProject(ctx, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeValidation, "project has related records")
		}
		return err
	}
	return nil
}

func ensureOwnerMember(members []model.ProjectUser, ownerID int64, projectID int64) []model.ProjectUser {
	for i := range members {
		if members[i].UserID == ownerID {
			members[i].Role = string(RoleAdmin)
			return members
		}
	}
	return append(members, model.ProjectUser{
		KanbanProjectID: projectID,
		UserID:          ownerID,
		Role:            string(RoleAdmin),
	})
}

func (s *ProjectService) GetNavProjectsForUser(ctx context.Context) ([]*dto.NavProjectResponse, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, accessDenied()
	}

	navProjects, err := s.repo.GetNavProjectsForUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]*dto.NavProjectResponse, 0, len(navProjects))
	for _, p := range navProjects {
		resp = append(resp, dto.MapNavProjectResponse(p, user.ID))
	}
	return resp, nil
}
