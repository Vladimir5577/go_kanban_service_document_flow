package service

import (
	"context"
	"unicode/utf8"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const maxCommentAttachments = 8

type CommentAttachmentServiceInterface interface {
	GetAttachment(ctx context.Context, cardID, commentID, id int64) (*model.CommentAttachment, error)
	CreateAttachment(ctx context.Context, cardID, commentID int64, req dto.CreateCommentAttachmentRequest) (*model.CommentAttachment, error)
	DeleteAttachment(ctx context.Context, cardID, commentID, id int64) error
}

// CommentAttachmentService — файлы комментариев. Права те же, что на сам
// комментарий: смотреть может viewer проекта, менять — только автор.
type CommentAttachmentService struct {
	repo       repository.CommentAttachmentRepositoryInterface
	commentSvc *CommentService
	History    HistoryLogger
}

func NewCommentAttachmentService(
	repo repository.CommentAttachmentRepositoryInterface,
	commentSvc *CommentService,
) *CommentAttachmentService {
	return &CommentAttachmentService{repo: repo, commentSvc: commentSvc}
}

func (s *CommentAttachmentService) GetAttachment(ctx context.Context, cardID, commentID, id int64) (*model.CommentAttachment, error) {
	if _, _, err := s.commentSvc.RequireCardComment(ctx, cardID, commentID); err != nil {
		return nil, err
	}
	return s.attachmentOf(ctx, commentID, id)
}

func (s *CommentAttachmentService) attachmentOf(ctx context.Context, commentID, id int64) (*model.CommentAttachment, error) {
	att, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeAttachmentNotFound)
	}
	if att.CommentID != commentID {
		return nil, apperr.New(apperr.CodeAttachmentNotFound, "attachment not found")
	}
	return att, nil
}

func (s *CommentAttachmentService) CreateAttachment(ctx context.Context, cardID, commentID int64, req dto.CreateCommentAttachmentRequest) (*model.CommentAttachment, error) {
	_, acc, err := s.commentSvc.RequireOwnComment(ctx, cardID, commentID)
	if err != nil {
		return nil, err
	}

	if utf8.RuneCountInString(req.Filename) > maxAttachmentFilenameLength {
		return nil, apperr.New(apperr.CodeFilenameTooLong, "filename too long")
	}

	existing, err := s.repo.GetByComment(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= maxCommentAttachments {
		return nil, apperr.New(apperr.CodeCommentAttachmentLimit, "maximum number of comment attachments reached")
	}

	created, err := s.repo.Create(ctx, &model.CommentAttachment{
		Filename:    req.Filename,
		StorageKey:  req.StorageKey,
		ContentType: req.ContentType,
		SizeBytes:   req.SizeBytes,
		CommentID:   commentID,
	})
	if err != nil {
		return nil, err
	}
	// Файлы, улетевшие вместе с отправкой комментария, накрывает его comment.created.
	if !req.SkipHistory {
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   acc.ProjectID,
			Action:      "attachment.created",
			EntityType:  "comment_attachment",
			EntityID:    created.ID,
			CardID:      cardID,
			EntityTitle: created.Filename,
		})
	}
	s.commentSvc.PublishCommentChanged(ctx, cardID, commentID)
	return created, nil
}

func (s *CommentAttachmentService) DeleteAttachment(ctx context.Context, cardID, commentID, id int64) error {
	c, acc, err := s.commentSvc.RequireOwnComment(ctx, cardID, commentID)
	if err != nil {
		return err
	}
	att, err := s.attachmentOf(ctx, commentID, id)
	if err != nil {
		return err
	}

	if c.Body == "" {
		files, err := s.repo.GetByComment(ctx, commentID)
		if err != nil {
			return err
		}
		// Последний файл комментария без текста: иначе останется пустое сообщение.
		if len(files) <= 1 {
			return apperr.New(apperr.CodeCommentBodyRequired, "comment body required")
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "attachment.deleted",
		EntityType:  "comment_attachment",
		EntityID:    att.ID,
		CardID:      cardID,
		EntityTitle: att.Filename,
	})
	s.commentSvc.PublishCommentChanged(ctx, cardID, commentID)
	return nil
}
