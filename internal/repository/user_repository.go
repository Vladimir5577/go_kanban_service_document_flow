package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/model"
)

type UserRepositoryInterface interface {
	LoginCheck(ctx context.Context) (*model.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error)
}

type UserRepository struct {
	Db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		Db: db,
	}
}

func (r *UserRepository) LoginCheck(ctx context.Context) (*model.User, error) {
	return &model.User{}, nil
}

func (r *UserRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := `SELECT id, login, lastname, firstname, patronymic, avatar_name FROM users WHERE id = ANY($1)`
	rows, err := r.Db.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		err := rows.Scan(&u.ID, &u.Login, &u.Lastname, &u.Firstname, &u.Patronymic, &u.AvatarName)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}
