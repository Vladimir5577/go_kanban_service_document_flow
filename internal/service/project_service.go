package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ProjectServiceInterface interface {
	GetAllProjects(ctx context.Context) ([]model.Project, error)
	CreateProject(ctx context.Context, req dto.CreateProjectRequest) (*model.Project, error)
	GetProject(ctx context.Context, id int64) (*dto.ProjectResponse, error)
	UpdateProject(ctx context.Context, id int64, req dto.UpdateProjectRequest) (*model.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	GetNavProjectsForUser(ctx context.Context) ([]*dto.NavProjectResponse, error)
}

type ProjectService struct {
	repo       repository.ProjectRepositoryInterface
	boardRepo  repository.BoardRepositoryInterface
	memberRepo repository.ProjectMemberRepositoryInterface
	userRepo   repository.UserRepositoryInterface
	permSvc    *PermissionService
}

func NewProjectService(
	repo repository.ProjectRepositoryInterface,
	boardRepo repository.BoardRepositoryInterface,
	memberRepo repository.ProjectMemberRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	permSvc *PermissionService,
) *ProjectService {
	return &ProjectService{
		repo:       repo,
		boardRepo:  boardRepo,
		memberRepo: memberRepo,
		userRepo:   userRepo,
		permSvc:    permSvc,
	}
}

func (s *ProjectService) GetAllProjects(ctx context.Context) ([]model.Project, error) {
	return s.repo.GetAllProjects(ctx)
}

func (s *ProjectService) CreateProject(ctx context.Context, req dto.CreateProjectRequest) (*model.Project, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrForbidden
	}

	p := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     user.ID,
		CreatedByID: &user.ID,
	}
	created, err := s.repo.CreateProject(ctx, p)
	if err != nil {
		return nil, err
	}

	err = s.memberRepo.ReplaceMembers(ctx, created.ID, []model.ProjectUser{
		{
			KanbanProjectID: created.ID,
			UserID:          user.ID,
			Role:            string(RoleAdmin),
		},
	})
	if err != nil {
		return nil, err
	}
	return created, nil
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
			memResp.AvatarUrl = u.AvatarName
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
		return nil, err
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	return s.repo.UpdateProject(ctx, p)
}

func (s *ProjectService) DeleteProject(ctx context.Context, id int64) error {
	if err := s.permSvc.RequireRole(ctx, id, RoleAdmin); err != nil {
		return err
	}
	err := s.repo.DeleteProject(ctx, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeValidation, "Нельзя удалить проект, пока в нем есть доски или участники.")
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
		return nil, apperr.ErrForbidden
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
