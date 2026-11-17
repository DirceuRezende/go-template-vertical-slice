// Package postgres implementa user.Repository sobre PostgreSQL (pgx).
package postgres

import (
	"context"
	"errors"
	"uuid"

	"users-api/internal/user"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const emailUniqueIndex = "users_email_unique_idx"

const userColumns = "id::text, name, email, password, age, created_at, updated_at"

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// mapError traduz a violação do índice único de email em user.ErrEmailTaken.
func mapError(err error) error {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok &&
		pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == emailUniqueIndex {
		return user.ErrEmailTaken
	}
	return err
}

func scanUser(row pgx.Row) (*user.User, error) {
	var (
		u  user.User
		id string
	)

	if err := row.Scan(&id, &u.Name, &u.Email, &u.Password, &u.Age, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}

	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	u.ID = parsed

	return &u, nil
}

func (r *Repository) Save(ctx context.Context, u user.User) (*user.User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (id, name, email, password, age, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+userColumns,
		u.ID.String(), u.Name, u.Email, u.Password, u.Age, u.CreatedAt, u.UpdatedAt)

	saved, err := scanUser(row)
	return saved, mapError(err)
}

func (r *Repository) Update(ctx context.Context, u user.User) (*user.User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users
		 SET name = $2, email = $3, age = $4, updated_at = now()
		 WHERE id = $1
		 RETURNING `+userColumns,
		u.ID.String(), u.Name, u.Email, u.Age)

	updated, err := scanUser(row)
	return updated, mapError(err)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *Repository) FindOne(ctx context.Context, id uuid.UUID) (*user.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id.String())

	return scanUser(row)
}

func (r *Repository) FindAll(ctx context.Context) ([]*user.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*user.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}

	return users, rows.Err()
}
