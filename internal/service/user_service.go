package service

import (
	"context"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type UserServiceInterface interface {
	LoginCheck(ctx context.Context) (*model.User, error)
}

type UserService struct {
	repo repository.UserRepositoryInterface
}

func NewUserService(repo repository.UserRepositoryInterface) *UserService {
	return &UserService{
		repo: repo,
	}
}

func (s *UserService) LoginCheck(ctx context.Context) (*model.User, error) {
	return s.repo.LoginCheck(ctx)
}
