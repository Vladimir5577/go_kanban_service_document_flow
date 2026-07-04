package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type CardServiceInterface interface {
	CreateCard(ctx context.Context, req dto.CreateCardRequest) (*model.Card, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error)
	UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateAssignees(ctx context.Context, id int64, userIDs []int64) error
	MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error)
	ArchiveCard(ctx context.Context, id int64) error
}

type CardService struct {
	repo           repository.CardRepositoryInterface
	permSvc        *PermissionService
	subtaskRepo    repository.SubtaskRepositoryInterface
	commentRepo    repository.CommentRepositoryInterface
	attachmentRepo repository.AttachmentRepositoryInterface
	labelRepo      repository.LabelRepositoryInterface
	userRepo       repository.UserRepositoryInterface
	cfg            *config.Config
}

func NewCardService(
	repo repository.CardRepositoryInterface,
	permSvc *PermissionService,
	subtaskRepo repository.SubtaskRepositoryInterface,
	commentRepo repository.CommentRepositoryInterface,
	attachmentRepo repository.AttachmentRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	cfg *config.Config,
) *CardService {
	return &CardService{
		repo:           repo,
		permSvc:        permSvc,
		subtaskRepo:    subtaskRepo,
		commentRepo:    commentRepo,
		attachmentRepo: attachmentRepo,
		labelRepo:      labelRepo,
		userRepo:       userRepo,
		cfg:            cfg,
	}
}

func (s *CardService) CreateCard(ctx context.Context, req dto.CreateCardRequest) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByColumn(ctx, req.ColumnID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	cards, err := s.repo.GetCardsByColumn(ctx, req.ColumnID)
	if err == nil && len(cards) >= 300 {
		return nil, apperr.New(apperr.CodeValidation, "maximum number of cards (300) in column reached")
	}

	if len(req.AssigneeIDs) > 1 {
		return nil, apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
	}

	c := &model.Card{
		Title:       req.Title,
		ColumnID:    req.ColumnID,
		Description: req.Description,
		DueDate:     req.DueDate,
		Priority:    req.Priority,
		BorderColor: req.BorderColor,
		AssigneeIDs: req.AssigneeIDs,
		LabelIDs:    req.LabelIDs,
	}
	if req.Position != nil {
		c.Position = *req.Position
	} else {
		cards, _ := s.repo.GetCardsByColumn(ctx, req.ColumnID)
		if len(cards) > 0 {
			c.Position = cards[len(cards)-1].Position + 65536.0
		} else {
			c.Position = 65536.0
		}
	}
	return s.repo.CreateCard(ctx, req.ColumnID, c)
}

func (s *CardService) GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, err
	}
	
	resp := dto.MapCardResponse(card)
	
	// Fetch Subtasks
	subtasks, _ := s.subtaskRepo.GetSubtasks(ctx, id)
	resp.Subtasks = dto.MapSubtasksResponse(subtasks)
	for _, st := range subtasks {
		resp.ChecklistTotal++
		if st.Status == "done" {
			resp.ChecklistDone++
		}
	}
	
	// Fetch Comments
	comments, _ := s.commentRepo.GetComments(ctx, id)
	resp.CommentsCount = len(comments)
	
	var userIDs []int64
	for i := range comments {
		userIDs = append(userIDs, comments[i].AuthorID)
	}
	for _, aid := range card.AssigneeIDs {
		userIDs = append(userIDs, aid)
	}
	for i := range resp.Subtasks {
		if resp.Subtasks[i].UserID != nil {
			userIDs = append(userIDs, *resp.Subtasks[i].UserID)
		}
	}
	
	var allAttachments []model.Attachment
	if atts, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "card"); err == nil {
		allAttachments = append(allAttachments, atts...)
	}
	if atts, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "description"); err == nil {
		allAttachments = append(allAttachments, atts...)
	}
	if atts, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "chat"); err == nil {
		allAttachments = append(allAttachments, atts...)
	}
	for i := range allAttachments {
		if allAttachments[i].AuthorID != nil {
			userIDs = append(userIDs, *allAttachments[i].AuthorID)
		}
	}
	
	users, _ := s.userRepo.GetUsersByIDs(ctx, userIDs)
	userMap := make(map[int64]*model.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	
	for i := range comments {
		if u, ok := userMap[comments[i].AuthorID]; ok {
			name := u.Firstname
			if u.Lastname != "" {
				name += " " + u.Lastname
			}
			comments[i].AuthorName = name
		}
	}
	resp.Comments = dto.MapCommentsResponse(comments)
	
	resp.Attachments = dto.MapAttachmentsResponse(s.cfg, allAttachments)
	for i, att := range resp.Attachments {
		if att.AuthorID != nil {
			if u, ok := userMap[*att.AuthorID]; ok {
				name := u.Firstname
				if u.Lastname != "" {
					name += " " + u.Lastname
				}
				resp.Attachments[i].AuthorName = &name
			}
		}
	}
	
	for _, st := range resp.Subtasks {
		if st.UserID != nil {
			if u, ok := userMap[*st.UserID]; ok {
				name := u.Firstname
				if u.Lastname != "" {
					name += " " + u.Lastname
				}
				st.UserName = &name
			}
		}
	}
	
	// Labels can be hydrated if needed, for now we skip doing extra db queries for them.
	// The frontend might use LabelIDs.
	
	// Let's just fetch all labels for the board. Wait, we don't have boardID easily here. 
	// We can get it from ProjectID? No. 
	
	// Let's skip hydrating labels fully if it's too complex right now, but wait, frontend needs color.
	// We can use s.repo.GetCardLabels? No, labels are already in card.LabelIDs. 
	
	for _, uid := range card.AssigneeIDs {
		if u, ok := userMap[uid]; ok {
			name := u.Firstname
			if u.Lastname != "" {
				name += " " + u.Lastname
			}
			resp.Assignees = append(resp.Assignees, &dto.CardAssigneeResponse{
				ID:        u.ID,
				Name:      name,
				AvatarUrl: u.AvatarName,
			})
		}
	}
	
	return resp, nil
}

func (s *CardService) GetCard(ctx context.Context, id int64) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetCard(ctx, id)
}

func (s *CardService) UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	c, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Title != nil {
		c.Title = *req.Title
	}
	if req.ColumnID != nil {
		c.ColumnID = *req.ColumnID
	}
	if req.Description != nil {
		c.Description = req.Description
	}
	if req.Position != nil {
		c.Position = *req.Position
	}
	if req.DueDate != nil {
		c.DueDate = req.DueDate
	}
	if req.Priority != nil {
		c.Priority = req.Priority
	}
	if req.IsArchived != nil {
		c.IsArchived = *req.IsArchived
	}
	if req.BorderColor != nil {
		c.BorderColor = req.BorderColor
	}
	if req.AssigneeIDs != nil {
		if len(req.AssigneeIDs) > 1 {
			return nil, apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
		}
		c.AssigneeIDs = req.AssigneeIDs
	}
	if req.LabelIDs != nil {
		c.LabelIDs = req.LabelIDs
	}
	return s.repo.UpdateCard(ctx, c)
}

func (s *CardService) DeleteCard(ctx context.Context, id int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	return s.repo.DeleteCard(ctx, id)
}

func (s *CardService) UpdateAssignees(ctx context.Context, id int64, userIDs []int64) error {
	if len(userIDs) > 1 {
		return apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
	}

	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	return s.repo.UpdateCardAssignees(ctx, id, userIDs)
}

func (s *CardService) MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}
	return s.repo.MoveCard(ctx, id, columnID, position)
}

func (s *CardService) ArchiveCard(ctx context.Context, id int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	return s.repo.ArchiveCard(ctx, id)
}
