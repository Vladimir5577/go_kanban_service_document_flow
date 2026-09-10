package service

import (
	"context"
	"unicode/utf8"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

// maxAttachmentFilenameLength matches the filename column width (VARCHAR(512)).
const maxAttachmentFilenameLength = 512

type AttachmentServiceInterface interface {
	GetAttachments(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error)
	GetAttachment(ctx context.Context, cardID, id int64, minRole Role) (*model.Attachment, error)
	CreateAttachment(ctx context.Context, cardID int64, req dto.CreateAttachmentRequest) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, attachment *model.Attachment) error
}

type AttachmentService struct {
	repo              repository.AttachmentRepositoryInterface
	permSvc           *PermissionService
	realtimePublisher *KanbanRealtimePublisher
	userRepo          repository.UserRepositoryInterface
	History           HistoryLogger
}

func NewAttachmentService(
	repo repository.AttachmentRepositoryInterface,
	permSvc *PermissionService,
	realtimePublisher *KanbanRealtimePublisher,
	userRepo repository.UserRepositoryInterface,
) *AttachmentService {
	return &AttachmentService{
		repo:              repo,
		permSvc:           permSvc,
		realtimePublisher: realtimePublisher,
		userRepo:          userRepo,
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
		return nil, withNotFoundCode(err, apperr.CodeAttachmentNotFound)
	}
	if att.CardID != cardID {
		return nil, apperr.New(apperr.CodeAttachmentNotFound, "attachment not found")
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

	if utf8.RuneCountInString(req.Filename) > maxAttachmentFilenameLength {
		return nil, apperr.New(apperr.CodeFilenameTooLong, "filename too long")
	}

	attachments, err := s.repo.GetAttachmentsByCard(ctx, cardID, req.Context)
	if err == nil && len(attachments) >= 16 {
		return nil, apperr.New(apperr.CodeAttachmentLimitReached, "maximum number of attachments (16) per context reached")
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
	if err == nil && created != nil {
		s.populateAuthorName(ctx, created)
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "attachment.created",
			EntityType:  "attachment",
			EntityID:    created.ID,
			CardID:      cardID,
			EntityTitle: created.Filename,
		})
	}
	if err == nil && created != nil && created.Context == "chat" && s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			patch, err := s.realtimePublisher.BuildCommentsCount(ctx, cardID)
			if err != nil {
				return err
			}
			return s.realtimePublisher.PublishCardPatchByID(ctx, cardID, patch, realtimeSenderID(ctx))
		})
	}
	return created, err
}

func (s *AttachmentService) DeleteAttachment(ctx context.Context, attachment *model.Attachment) error {
	if attachment == nil {
		return apperr.New(apperr.CodeAttachmentNotFound, "attachment not found")
	}
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, attachment.CardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}

	err = s.repo.DeleteAttachment(ctx, attachment.ID)
	if err == nil {
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "attachment.deleted",
			EntityType:  "attachment",
			EntityID:    attachment.ID,
			CardID:      attachment.CardID,
			EntityTitle: attachment.Filename,
		})
	}
	if err == nil && attachment.Context == "chat" && s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			patch, err := s.realtimePublisher.BuildCommentsCount(ctx, attachment.CardID)
			if err != nil {
				return err
			}
			return s.realtimePublisher.PublishCardPatchByID(ctx, attachment.CardID, patch, realtimeSenderID(ctx))
		})
	}
	return err
}

func (s *AttachmentService) populateAuthorName(ctx context.Context, a *model.Attachment) {
	if a.AuthorID == nil {
		return
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, []int64{*a.AuthorID})
	if err != nil || len(users) == 0 {
		return
	}
	name := dto.UserDisplayName(users[0])
	a.AuthorName = &name
}
