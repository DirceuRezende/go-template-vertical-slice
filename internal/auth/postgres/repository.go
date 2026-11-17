// Package postgres implementa auth.Repository sobre PostgreSQL (pgx).
package postgres

import (
	"context"
	"errors"
	"time"
	"uuid"

	"users-api/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const insertToken = `INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, token_version, expires_at, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)`

func (r *Repository) Create(ctx context.Context, rt auth.RefreshToken) error {
	_, err := r.pool.Exec(ctx, insertToken,
		rt.ID.String(), rt.UserID.String(), rt.FamilyID.String(), rt.TokenHash, rt.TokenVersion, rt.ExpiresAt, rt.CreatedAt)
	return err
}

func (r *Repository) Rotate(ctx context.Context, oldHash string, next auth.RefreshToken, now time.Time) (auth.RefreshToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return auth.RefreshToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE: duas rotações simultâneas do mesmo token se serializam aqui; a
	// segunda enxerga used_at preenchido e cai no caminho de reuso.
	var (
		userID, familyID string
		version          int
		expiresAt        time.Time
		usedAt, revoked  *time.Time
	)
	err = tx.QueryRow(ctx,
		`SELECT user_id::text, family_id::text, token_version, expires_at, used_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = $1 FOR UPDATE`, oldHash).
		Scan(&userID, &familyID, &version, &expiresAt, &usedAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}
	if err != nil {
		return auth.RefreshToken{}, err
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return auth.RefreshToken{}, err
	}
	fid, err := uuid.Parse(familyID)
	if err != nil {
		return auth.RefreshToken{}, err
	}

	if revoked != nil {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}

	if usedAt != nil {
		// A revogação precisa ser confirmada mesmo devolvendo erro ao chamador.
		if _, err := tx.Exec(ctx,
			`UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1 AND revoked_at IS NULL`,
			familyID, now); err != nil {
			return auth.RefreshToken{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return auth.RefreshToken{}, err
		}
		return auth.RefreshToken{UserID: uid, FamilyID: fid}, auth.ErrTokenReuse
	}

	if !expiresAt.After(now) {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}

	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET used_at = $2 WHERE token_hash = $1`, oldHash, now); err != nil {
		return auth.RefreshToken{}, err
	}

	next.UserID, next.FamilyID, next.TokenVersion = uid, fid, version
	if _, err := tx.Exec(ctx, insertToken,
		next.ID.String(), next.UserID.String(), next.FamilyID.String(), next.TokenHash, next.TokenVersion, next.ExpiresAt, next.CreatedAt); err != nil {
		return auth.RefreshToken{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return auth.RefreshToken{}, err
	}
	return next, nil
}

func (r *Repository) RevokeFamily(ctx context.Context, hash string, now time.Time) (uuid.UUID, error) {
	var userID string
	err := r.pool.QueryRow(ctx,
		`WITH target AS (SELECT user_id, family_id FROM refresh_tokens WHERE token_hash = $1),
		      revoked AS (
		        UPDATE refresh_tokens SET revoked_at = $2
		        WHERE family_id = (SELECT family_id FROM target) AND revoked_at IS NULL
		      )
		 SELECT user_id::text FROM target`, hash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil(), auth.ErrInvalidRefreshToken
	}
	if err != nil {
		return uuid.Nil(), err
	}
	return uuid.Parse(userID)
}
