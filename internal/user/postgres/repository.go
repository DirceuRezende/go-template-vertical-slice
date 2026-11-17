// Package postgres implementa user.Repository sobre PostgreSQL (pgx).
package postgres

import (
	"context"
	"errors"
	"slices"
	"uuid"

	"users-api/internal/user"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const emailUniqueIndex = "users_email_unique_idx"

const userColumns = `id::text, name, email, password, age, created_at, updated_at,
	COALESCE((SELECT array_agg(role ORDER BY role) FROM user_roles WHERE user_id = users.id), '{}'),
	token_version`

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

	if err := row.Scan(&id, &u.Name, &u.Email, &u.Password, &u.Age, &u.CreatedAt, &u.UpdatedAt, &u.Roles, &u.TokenVersion); err != nil {
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

// inTx executa fn numa transação, com rollback em qualquer erro.
func (r *Repository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Save(ctx context.Context, u user.User) (*user.User, error) {
	var saved *user.User

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (id, name, email, password, age, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			u.ID.String(), u.Name, u.Email, u.Password, u.Age, u.CreatedAt, u.UpdatedAt); err != nil {
			return mapError(err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, u.ID.String(), user.RoleUser); err != nil {
			return err
		}

		var err error
		saved, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, u.ID.String()))
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
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

// lockAdmins trava as linhas de admin (em ordem fixa, para não gerar deadlock) e
// devolve os ids. Toda operação que pode reduzir o número de admins passa por aqui,
// o que serializa essas operações e evita que duas deixem o sistema sem admin.
func lockAdmins(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT user_id::text FROM user_roles WHERE role = $1 ORDER BY user_id FOR UPDATE`, user.RoleAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		admins, err := lockAdmins(ctx, tx)
		if err != nil {
			return err
		}
		if len(admins) == 1 && admins[0] == id.String() {
			return user.ErrLastAdmin
		}

		tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id.String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return user.ErrNotFound
		}
		return nil
	})
}

func (r *Repository) FindOne(ctx context.Context, id uuid.UUID) (*user.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id.String())

	return scanUser(row)
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email)

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

// UpdatePassword incrementa token_version: os refresh tokens da versão anterior
// deixam de valer.
func (r *Repository) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password = $2, token_version = token_version + 1, updated_at = now() WHERE id = $1`,
		id.String(), hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *Repository) SetRoles(ctx context.Context, id uuid.UUID, roles []string) (*user.User, error) {
	var updated *user.User

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		admins, err := lockAdmins(ctx, tx)
		if err != nil {
			return err
		}

		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id.String()).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return user.ErrNotFound
		}

		losesAdmin := !slices.Contains(roles, user.RoleAdmin)
		if losesAdmin && len(admins) == 1 && admins[0] == id.String() {
			return user.ErrLastAdmin
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO roles (name) SELECT unnest($1::text[]) ON CONFLICT DO NOTHING`, roles); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM user_roles WHERE user_id = $1 AND role <> ALL($2::text[])`, id.String(), roles); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`,
			id.String(), roles); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE users SET updated_at = now() WHERE id = $1`, id.String()); err != nil {
			return err
		}

		updated, err = scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id.String()))
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}
