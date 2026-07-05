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
	GetAttachment(ctx context.Context, cardID, id int64, minRole Role) (*model.Attachment, error)
	CreateAttachment(ctx context.Context, cardID int64, req dto.CreateAttachmentRequest) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, attachment *model.Attachment) error
}

type AttachmentService struct {
	repo         repository.AttachmentRepositoryInterface
	permSvc      *PermissionService
	activityRepo repository.ActivityRepositoryInterface
}

func NewAttachmentService(repo repository.AttachmentRepositoryInterface, permSvc *PermissionService, activityRepo repository.ActivityRepositoryInterface) *AttachmentService {
	return &AttachmentService{
		repo:         repo,
		permSvc:      permSvc,
		activityRepo: activityRepo,
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

func (s *AttachmentService) GetAttachment(ctx context.Context, cardID, id int64, minRole Role) (*model.Attachment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, minRole); err != nil {
		return nil, err
	}

	att, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return nil, err
	}
	if att.CardID != cardID {
		return nil, apperr.ErrNotFound
	}
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
	created, err := s.repo.CreateAttachment(ctx, cardID, a)
	if err == nil && created != nil && created.Context != "chat" {
		s.logActivity(ctx, cardID, "attachment_added", nil, &created.Filename)
	}
	return created, err
}

func (s *AttachmentService) DeleteAttachment(ctx context.Context, attachment *model.Attachment) error {
	if attachment == nil {
		return apperr.ErrNotFound
	}
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, attachment.CardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}

	err = s.repo.DeleteAttachment(ctx, attachment.ID)
	if err == nil && attachment.Context != "chat" {
		s.logActivity(ctx, attachment.CardID, "attachment_removed", &attachment.Filename, nil)
	}
	return err
}

func (s *AttachmentService) logActivity(ctx context.Context, cardID int64, action string, oldValue, newValue *string) {
	_ = s.activityRepo.LogActivity(ctx, cardID, currentUserID(ctx), action, oldValue, newValue)
}
