package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/client"
	"go_kanban_service/internal/model"
)

type UserRepositoryInterface interface {
	LoginCheck(ctx context.Context) (*model.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error)
}

type UserRepository struct {
	Db            *pgxpool.Pool
	SymfonyClient client.SymfonyClientInterface
}

func NewUserRepository(db *pgxpool.Pool, symfonyClient client.SymfonyClientInterface) *UserRepository {
	return &UserRepository{
		Db:            db,
		SymfonyClient: symfonyClient,
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

	// Проверяем, всех ли пользователей нашли
	if len(users) < len(ids) {
		foundIDs := make(map[int64]bool, len(users))
		for _, u := range users {
			foundIDs[u.ID] = true
		}

		var missingIDs []int64
		for _, id := range ids {
			if !foundIDs[id] {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 && r.SymfonyClient != nil {
			fetchedUsers, err := r.SymfonyClient.FetchUsersByIDs(ctx, missingIDs)
			if err == nil && len(fetchedUsers) > 0 {
				// Сохраняем в локальную БД
				if err := r.UpsertUsers(ctx, fetchedUsers); err == nil {
					users = append(users, fetchedUsers...)
				}
			}
		}
	}

	return users, nil
}

func (r *UserRepository) UpsertUsers(ctx context.Context, users []model.User) error {
	if len(users) == 0 {
		return nil
	}

	// Используем транзакцию для создания temp таблицы и bulk upsert
	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Создаём временную таблицу (автоматически удалится при commit/rollback)
	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE users_tmp (LIKE users INCLUDING DEFAULTS)
		ON COMMIT DROP
	`)
	if err != nil {
		return err
	}

	// 2. Bulk-вставка через COPY протокол (очень быстро)
	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"users_tmp"},
		[]string{"id", "login", "lastname", "firstname", "patronymic", "avatar_name", "deleted_at", "synced_at"},
		pgx.CopyFromSlice(len(users), func(i int) ([]any, error) {
			u := users[i]
			return []any{
				u.ID,
				u.Login,
				u.Lastname,
				u.Firstname,
				u.Patronymic,
				u.AvatarName,
				u.DeletedAt,
				time.Now(),
			}, nil
		}),
	)
	if err != nil {
		return err
	}

	// 3. Upsert из temp таблицы в основную
	_, err = tx.Exec(ctx, `
		INSERT INTO users
		SELECT * FROM users_tmp
		ON CONFLICT (id) DO UPDATE SET
			login = EXCLUDED.login,
			lastname = EXCLUDED.lastname,
			firstname = EXCLUDED.firstname,
			patronymic = EXCLUDED.patronymic,
			avatar_name = EXCLUDED.avatar_name,
			deleted_at = EXCLUDED.deleted_at,
			synced_at = EXCLUDED.synced_at
	`)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *UserRepository) MarkUserDeleted(ctx context.Context, userID int64, deletedAt time.Time) error {
	_, err := r.Db.Exec(ctx, `
		UPDATE users
		SET deleted_at = $2,
			synced_at = NOW()
		WHERE id = $1
	`, userID, deletedAt)
	return err
}
