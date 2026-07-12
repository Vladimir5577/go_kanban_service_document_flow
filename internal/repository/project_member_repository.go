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
	AddMember(ctx context.Context, projectID int64, member model.ProjectUser) error
	ReplaceMembers(ctx context.Context, projectID int64, members []model.ProjectUser) error
	UpdateMemberRole(ctx context.Context, projectID int64, userID int64, role string) error
	RemoveMember(ctx context.Context, projectID int64, userID int64) error

	// GetAdminUserIDs returns owner + users with KANBAN_ADMIN role for the project.
	// Used for notification recipient calculation (mirrors Symfony findAdminUsersByProject).
	GetAdminUserIDs(ctx context.Context, projectID int64) ([]int64, error)
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
		return nil, NormalizeError(err)
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

func (r *ProjectMemberRepository) AddMember(ctx context.Context, projectID int64, member model.ProjectUser) error {
	queries := dbgen.New(r.Db)
	params := dbgen.AddProjectMemberParams{
		KanbanProjectID: int32(projectID),
		UserID:          int32(member.UserID),
		Role:            member.Role,
		Position:        member.Position,
	}
	if member.FolderID != nil {
		params.FolderID = pgtype.Int4{Int32: int32(*member.FolderID), Valid: true}
	}
	return queries.AddProjectMember(ctx, params)
}

func (r *ProjectMemberRepository) ReplaceMembers(ctx context.Context, projectID int64, members []model.ProjectUser) error {
	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	queries := dbgen.New(tx)
	if err := queries.ReplaceProjectMembers(ctx, int32(projectID)); err != nil {
		return err
	}

	for _, m := range members {
		params := dbgen.AddProjectMemberParams{
			KanbanProjectID: int32(projectID),
			UserID:          int32(m.UserID),
			Role:            m.Role,
			Position:        m.Position,
		}
		if m.FolderID != nil {
			params.FolderID = pgtype.Int4{Int32: int32(*m.FolderID), Valid: true}
		}

		if err := queries.AddProjectMember(ctx, params); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ProjectMemberRepository) UpdateMemberRole(ctx context.Context, projectID int64, userID int64, role string) error {
	queries := dbgen.New(r.Db)
	return queries.UpdateProjectMemberRole(ctx, dbgen.UpdateProjectMemberRoleParams{
		KanbanProjectID: int32(projectID),
		UserID:          int32(userID),
		Role:            role,
	})
}

func (r *ProjectMemberRepository) RemoveMember(ctx context.Context, projectID int64, userID int64) error {
	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		DELETE FROM kanban_card_assignee ca
		USING kanban_card c
		JOIN kanban_column col ON c.column_id = col.id
		JOIN kanban_board b ON col.board_id = b.id
		WHERE ca.card_id = c.id
			AND ca.user_id = $2
			AND b.kanban_project_id = $1
	`, projectID, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE kanban_card_subtask st
		SET user_id = NULL
		FROM kanban_card c
		JOIN kanban_column col ON c.column_id = col.id
		JOIN kanban_board b ON col.board_id = b.id
		WHERE st.card_id = c.id
			AND st.user_id = $2
			AND b.kanban_project_id = $1
	`, projectID, userID); err != nil {
		return err
	}

	queries := dbgen.New(tx)
	if err := queries.RemoveProjectMember(ctx, dbgen.RemoveProjectMemberParams{
		KanbanProjectID: int32(projectID),
		UserID:          int32(userID),
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetAdminUserIDs returns the owner of the project plus all members with KANBAN_ADMIN role.
func (r *ProjectMemberRepository) GetAdminUserIDs(ctx context.Context, projectID int64) ([]int64, error) {
	var ownerID int64
	err := r.Db.QueryRow(ctx, `SELECT owner_id FROM kanban_project WHERE id = $1 AND deleted_at IS NULL`, projectID).Scan(&ownerID)
	if err != nil {
		return nil, NormalizeError(err)
	}

	admins := map[int64]bool{}
	if ownerID != 0 {
		admins[ownerID] = true
	}

	queries := dbgen.New(r.Db)
	members, err := queries.GetProjectMembers(ctx, int32(projectID))
	if err != nil {
		return nil, err
	}

	for _, m := range members {
		if m.Role == "KANBAN_ADMIN" {
			admins[int64(m.UserID)] = true
		}
	}

	result := make([]int64, 0, len(admins))
	for id := range admins {
		result = append(result, id)
	}
	return result, nil
}
