package postgres

import (
	"context"
	"errors"
	"fmt"

	"users-api/internal/user"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Save(ctx context.Context, p user.User) (*user.User, error) {
	const query = `
		INSERT INTO user (name, email, password, age)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, email, password, age, created_at, updated_at`

	var saved user.User
	err := r.db.QueryRow(ctx, query,
		p.Name, p.Name, p.Email, p.Password, p.Age,
	).Scan(&saved.ID, &saved.Name, &saved.Email, &saved.Password, &saved.Age, &saved.CreatedAt, &saved.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("save user: %w", err)
	}

	return &saved, nil
}

func (r *Repository) Update(ctx context.Context, p user.User) (*user.User, error) {
	const query = `
		UPDATE users
		SET name = $2, email = $3, age = $4, updated_at = $5
		WHERE id = $1
		RETURNING id, name, email, password, age, created_at, updated_at`

	var updated user.User
	err := r.db.QueryRow(ctx, query,
		p.ID, p.Name, p.Email, p.Age, p.UpdatedAt,
	).Scan(&updated.ID, &updated.Name, &updated.Email, &updated.Password, &updated.Age, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	return &updated, nil
}

func (r *Repository) Delete(ctx context.Context, id uint64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}

func (r *Repository) FindOne(ctx context.Context, id uint64) (*user.User, error) {
	const query = `
		SELECT id, name, email, password, age, created_at, updated_at
		FROM users
		WHERE id = $1`

	var found user.User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&found.ID, &found.Name, &found.Email, &found.Password, &found.Age, &found.CreatedAt, &found.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, fmt.Errorf("find user: %w", err)
	}

	return &found, nil
}

func (r *Repository) FindAll(ctx context.Context) ([]*user.User, error) {
	const query = `
		SELECT id, name, email, password, age, created_at, updated_at
		FROM users
		ORDER BY id`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("find all users: %w", err)
	}
	defer rows.Close()

	users := []*user.User{}

	for rows.Next() {
		var p user.User
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.Password, &p.Age, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find all users: %w", err)
	}

	return users, nil
}
