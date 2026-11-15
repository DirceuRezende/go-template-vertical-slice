package user

import (
	"context"
	"errors"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const userColumns = "id::text, name, email, password, age, created_at, updated_at"

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func scanUser(row pgx.Row) (*User, error) {
	var (
		u  User
		id string
	)

	if err := row.Scan(&id, &u.Name, &u.Email, &u.Password, &u.Age, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
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

func (r *PostgresRepository) Save(ctx context.Context, u User) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (id, name, email, password, age, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+userColumns,
		u.ID.String(), u.Name, u.Email, u.Password, u.Age, u.CreatedAt, u.UpdatedAt)

	return scanUser(row)
}

func (r *PostgresRepository) Update(ctx context.Context, u User) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users
		 SET name = $2, email = $3, age = $4, updated_at = now()
		 WHERE id = $1
		 RETURNING `+userColumns,
		u.ID.String(), u.Name, u.Email, u.Age)

	return scanUser(row)
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) FindOne(ctx context.Context, id uuid.UUID) (*User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id.String())

	return scanUser(row)
}

func (r *PostgresRepository) FindAll(ctx context.Context) ([]*User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}

	return users, rows.Err()
}
