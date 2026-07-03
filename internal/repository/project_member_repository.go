package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type ProjectMemberRepositoryInterface interface {
	GetMembers(ctx context.Context, projectID int64) ([]model.ProjectUser, error)
	GetProjectMember(ctx context.Context, projectID, userID int64) (*model.ProjectUser, error)
	ReplaceMembers(ctx context.Context, projectID int64, members []model.ProjectUser) error
	UpdateMemberRole(ctx context.Context, projectID int64, userID int64, role string) error
	RemoveMember(ctx context.Context, projectID int64, userID int64) error
}

type ProjectMemberRepository struct {
	Db *pgxpool.Pool
}

func NewProjectMemberRepository(db *pgxpool.Pool) *ProjectMemberRepository {
	return &ProjectMemberRepository{
		Db: db,
	}
}

func (r *ProjectMemberRepository) GetMembers(ctx context.Context, projectID int64) ([]model.ProjectUser, error) {
	queries := dbgen.New(r.Db)
	dbMembers, err := queries.GetProjectMembers(ctx, int32(projectID))
	if err != nil {
		return nil, err
	}

	var members []model.ProjectUser
	for _, m := range dbMembers {
		member := model.ProjectUser{
			KanbanProjectID: int64(m.KanbanProjectID),
			UserID:          int64(m.UserID),
			Role:            m.Role,
		}
		if m.FolderID.Valid {
			v := int64(m.FolderID.Int32)
			member.FolderID = &v
		}
		members = append(members, member)
	}
	return members, nil
}

func (r *ProjectMemberRepository) GetProjectMember(ctx context.Context, projectID, userID int64) (*model.ProjectUser, error) {
	queries := dbgen.New(r.Db)
	dbMember, err := queries.GetProjectMember(ctx, dbgen.GetProjectMemberParams{
		KanbanProjectID: int32(projectID),
		UserID:          int32(userID),
	})
	if err != nil {
		return nil, err
	}

	return &model.ProjectUser{
		ID:              int64(dbMember.ID),
		KanbanProjectID: int64(dbMember.KanbanProjectID),
		UserID:          int64(dbMember.UserID),
		Role:            dbMember.Role,
		FolderID: func() *int64 {
			if dbMember.FolderID.Valid {
				v := int64(dbMember.FolderID.Int32)
				return &v
			}
			return nil
		}(),
		Position: dbMember.Position,
	}, nil
}

func (r *ProjectMemberRepository) ReplaceMembers(ctx context.Context, projectID int64, members []model.ProjectUser) error {
	queries := dbgen.New(r.Db)

	// В идеале использовать транзакции (tx), но пока оставим через r.Db
	if err := queries.ReplaceProjectMembers(ctx, int32(projectID)); err != nil {
		return err
	}

	for _, m := range members {
		params := dbgen.AddProjectMemberParams{
			KanbanProjectID: int32(projectID),
			UserID:          int32(m.UserID),
			Role:            m.Role,
		}
		if m.FolderID != nil {
			params.FolderID = pgtype.Int4{Int32: int32(*m.FolderID), Valid: true}
		}

		if err := queries.AddProjectMember(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProjectMemberRepository) UpdateMemberRole(ctx context.Context, projectID int64, userID int64, role string) error {
	return nil
}

func (r *ProjectMemberRepository) RemoveMember(ctx context.Context, projectID int64, userID int64) error {
	queries := dbgen.New(r.Db)
	return queries.RemoveProjectMember(ctx, dbgen.RemoveProjectMemberParams{
		KanbanProjectID: int32(projectID),
		UserID:          int32(userID),
	})
}
