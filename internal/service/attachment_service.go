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

const (
	// maxAttachmentFilenameLength matches the filename column width (VARCHAR(512)).
	maxAttachmentFilenameLength = 512
	maxCardAttachments          = 16
)

type AttachmentServiceInterface interface {
	GetAttachment(ctx context.Context, cardID, id int64, minRole Role) (*model.Attachment, error)
	CreateAttachment(ctx context.Context, cardID int64, req dto.CreateAttachmentRequest) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, attachment *model.Attachment) error
}

// AttachmentService — файлы карточки. Файлы комментариев — CommentAttachmentService.
type AttachmentService struct {
	repo     repository.AttachmentRepositoryInterface
	permSvc  *PermissionService
	userRepo repository.UserRepositoryInterface
	History  HistoryLogger
}

func NewAttachmentService(
	repo repository.AttachmentRepositoryInterface,
	permSvc *PermissionService,
	userRepo repository.UserRepositoryInterface,
) *AttachmentService {
	return &AttachmentService{
		repo:     repo,
		permSvc:  permSvc,
		userRepo: userRepo,
	}
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
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleEditor)
	if err != nil {
		return nil, err
	}

	if utf8.RuneCountInString(req.Filename) > maxAttachmentFilenameLength {
		return nil, apperr.New(apperr.CodeFilenameTooLong, "filename too long")
	}

	existing, err := s.repo.GetAttachmentsByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= maxCardAttachments {
		return nil, apperr.New(apperr.CodeAttachmentLimitReached, "maximum number of attachments reached")
	}

	var authorID *int64
	if user, ok := middleware.GetUser(ctx); ok {
		authorID = &user.ID
	}

	a := &model.Attachment{
		Filename:    req.Filename,
		StorageKey:  req.StorageKey,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		AuthorID:    authorID,
		CardID:      cardID,
	}
	created, err := s.repo.CreateAttachment(ctx, cardID, a)
	if err != nil {
		return nil, err
	}
	s.populateAuthorName(ctx, created)
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "attachment.created",
		EntityType:  "attachment",
		EntityID:    created.ID,
		CardID:      cardID,
		EntityTitle: created.Filename,
	})
	return created, nil
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

	if err := s.repo.DeleteAttachment(ctx, attachment.ID); err != nil {
		return err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   projectID,
		Action:      "attachment.deleted",
		EntityType:  "attachment",
		EntityID:    attachment.ID,
		CardID:      attachment.CardID,
		EntityTitle: attachment.Filename,
	})
	return nil
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
