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
	UpdateProjectPlacement(ctx context.Context, projectID int64, userID int64, folderID *int64, position float64) (*model.ProjectUser, error)
	RebalanceProjectPositions(ctx context.Context, userID int64, folderID *int64) (bool, error)
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
	dbMembers, err := queries.GetProjectMembers(ctx, projectID)
	if err != nil {
		return nil, NormalizeError(err)
	}

	var members []model.ProjectUser
	for _, m := range dbMembers {
		member := model.ProjectUser{
			KanbanProjectID: m.KanbanProjectID,
			UserID:          m.UserID,
			Role:            m.Role,
		}
		if m.FolderID.Valid {
			v := m.FolderID.Int64
			member.FolderID = &v
		}
		members = append(members, member)
	}
	return members, nil
}

func (r *ProjectMemberRepository) GetProjectMember(ctx context.Context, projectID, userID int64) (*model.ProjectUser, error) {
	queries := dbgen.New(r.Db)
	dbMember, err := queries.GetProjectMember(ctx, dbgen.GetProjectMemberParams{
		KanbanProjectID: projectID,
		UserID:          userID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	return mapDBProjectMember(dbMember.ID, dbMember.KanbanProjectID, dbMember.UserID, dbMember.Role, dbMember.FolderID, dbMember.Position), nil
}

func (r *ProjectMemberRepository) AddMember(ctx context.Context, projectID int64, member model.ProjectUser) error {
	queries := dbgen.New(r.Db)
	params := dbgen.AddProjectMemberParams{
		KanbanProjectID: projectID,
		UserID:          member.UserID,
		Role:            member.Role,
		Position:        member.Position,
	}
	if member.FolderID != nil {
		params.FolderID = pgtype.Int8{Int64: *member.FolderID, Valid: true}
	}
	return queries.AddProjectMember(ctx, params)
}

func (r *ProjectMemberRepository) ReplaceMembers(ctx context.Context, projectID int64, members []model.ProjectUser) error {
	return ExecTx(ctx, r.Db, func(q *dbgen.Queries) error {
		if err := q.ReplaceProjectMembers(ctx, projectID); err != nil {
			return err
		}

		for _, m := range members {
			params := dbgen.AddProjectMemberParams{
				KanbanProjectID: projectID,
				UserID:          m.UserID,
				Role:            m.Role,
				Position:        m.Position,
			}
			if m.FolderID != nil {
				params.FolderID = pgtype.Int8{Int64: *m.FolderID, Valid: true}
			}

			if err := q.AddProjectMember(ctx, params); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ProjectMemberRepository) UpdateMemberRole(ctx context.Context, projectID int64, userID int64, role string) error {
	queries := dbgen.New(r.Db)
	return queries.UpdateProjectMemberRole(ctx, dbgen.UpdateProjectMemberRoleParams{
		KanbanProjectID: projectID,
		UserID:          userID,
		Role:            role,
	})
}

func (r *ProjectMemberRepository) UpdateProjectPlacement(ctx context.Context, projectID int64, userID int64, folderID *int64, position float64) (*model.ProjectUser, error) {
	query := `
		UPDATE kanban_project_user
		SET folder_id = $1, position = $2
		WHERE kanban_project_id = $3 AND user_id = $4
		RETURNING id, kanban_project_id, user_id, role, folder_id, position`

	var folderParam pgtype.Int8
	if folderID != nil {
		folderParam = pgtype.Int8{Int64: *folderID, Valid: true}
	}

	var (
		id              int64
		kanbanProjectID int64
		memberUserID    int64
		role            string
		dbFolderID      pgtype.Int8
		dbPosition      float64
	)
	err := r.Db.QueryRow(ctx, query, folderParam, position, projectID, userID).Scan(
		&id,
		&kanbanProjectID,
		&memberUserID,
		&role,
		&dbFolderID,
		&dbPosition,
	)
	if err != nil {
		return nil, NormalizeError(err)
	}

	return mapDBProjectMember(id, kanbanProjectID, memberUserID, role, dbFolderID, dbPosition), nil
}

func (r *ProjectMemberRepository) RebalanceProjectPositions(ctx context.Context, userID int64, folderID *int64) (bool, error) {
	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return false, err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		SELECT id, position
		FROM kanban_project_user
		WHERE user_id = $1 AND folder_id IS NULL
		ORDER BY position ASC, id ASC`
	args := []any{userID}
	if folderID != nil {
		query = `
			SELECT id, position
			FROM kanban_project_user
			WHERE user_id = $1 AND folder_id = $2
			ORDER BY position ASC, id ASC`
		args = append(args, *folderID)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	type positionedProject struct {
		id       int64
		position float64
	}
	projects := make([]positionedProject, 0)
	for rows.Next() {
		var p positionedProject
		if err := rows.Scan(&p.id, &p.position); err != nil {
			return false, err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	needsRebalance := false
	for i := 1; i < len(projects); i++ {
		if projects[i].position-projects[i-1].position < 0.0001 {
			needsRebalance = true
			break
		}
	}
	if !needsRebalance {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		committed = true
		return false, nil
	}

	for i, p := range projects {
		if _, err := tx.Exec(ctx, `
			UPDATE kanban_project_user
			SET position = $1
			WHERE id = $2
		`, float64(i+1), p.id); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func (r *ProjectMemberRepository) RemoveMember(ctx context.Context, projectID int64, userID int64) error {
	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
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
		KanbanProjectID: projectID,
		UserID:          userID,
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
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
	members, err := queries.GetProjectMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}

	for _, m := range members {
		if m.Role == "KANBAN_ADMIN" {
			admins[m.UserID] = true
		}
	}

	result := make([]int64, 0, len(admins))
	for id := range admins {
		result = append(result, id)
	}
	return result, nil
}

func mapDBProjectMember(id int64, projectID int64, userID int64, role string, folderID pgtype.Int8, position float64) *model.ProjectUser {
	member := &model.ProjectUser{
		ID:              id,
		KanbanProjectID: projectID,
		UserID:          userID,
		Role:            role,
		Position:        position,
	}
	if folderID.Valid {
		v := folderID.Int64
		member.FolderID = &v
	}
	return member
}
