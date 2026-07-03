package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type AttachmentServiceInterface interface {
	GetAttachments(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error)
	GetAttachment(ctx context.Context, id int64) (*model.Attachment, error)
	CreateAttachment(ctx context.Context, cardID int64, req dto.CreateAttachmentRequest) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, id int64) error
}

type AttachmentService struct {
	repo    repository.AttachmentRepositoryInterface
	permSvc *PermissionService
}

func NewAttachmentService(repo repository.AttachmentRepositoryInterface, permSvc *PermissionService) *AttachmentService {
	return &AttachmentService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *AttachmentService) GetAttachments(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetAttachmentsByCard(ctx, cardID, contextStr)
}

func (s *AttachmentService) GetAttachment(ctx context.Context, id int64) (*model.Attachment, error) {
	att, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return nil, err
	}
	// Perm checks omitted for preview/download to work without tokens in img tags
	return att, nil
}

func (s *AttachmentService) CreateAttachment(ctx context.Context, cardID int64, req dto.CreateAttachmentRequest) (*model.Attachment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	attachments, err := s.repo.GetAttachmentsByCard(ctx, cardID, req.Context)
	if err == nil && len(attachments) >= 16 {
		return nil, apperr.New(apperr.CodeValidation, "maximum number of attachments (16) per context reached")
	}
	
	var authorID *int64
	user, ok := middleware.GetUser(ctx)
	if ok {
		authorID = &user.ID
	}

	a := &model.Attachment{
		Filename:    req.Filename,
		StorageKey:  req.StorageKey,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		Context:     req.Context,
		AuthorID:    authorID,
		CardID:      cardID,
	}
	return s.repo.CreateAttachment(ctx, cardID, a)
}

func (s *AttachmentService) DeleteAttachment(ctx context.Context, id int64) error {
	return s.repo.DeleteAttachment(ctx, id)
}
