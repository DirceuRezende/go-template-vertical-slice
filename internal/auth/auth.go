// Package auth implementa login, renovação de sessão (refresh token), logout e
// troca de senha. O access token é um JWT curto (plataform/token); o refresh
// token é opaco, de uso único e guardado só como hash.
package auth

import (
	"context"
	"errors"
	"time"
	"uuid"

	"users-api/internal/user"
)

var (
	ErrInvalidCredentials  = errors.New("Credenciais inválidas")
	ErrInvalidRefreshToken = errors.New("Refresh token inválido")
	ErrWrongPassword       = errors.New("Senha atual incorreta")

	// ErrTokenReuse é interno ao módulo: um refresh token já usado foi apresentado
	// de novo (possível roubo). O repositório já revogou a família; o service
	// responde ao cliente com ErrInvalidRefreshToken.
	ErrTokenReuse = errors.New("refresh token reutilizado")
)

// RefreshToken é uma credencial de renovação. Os tokens de uma mesma sessão
// (cadeia de rotações) compartilham o FamilyID.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	TokenHash string
	// TokenVersion é o users.token_version na emissão; se a senha mudou desde
	// então, o refresh é recusado.
	TokenVersion int
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UsedAt       *time.Time
	RevokedAt    *time.Time
}

// Repository persiste refresh tokens.
type Repository interface {
	// Create guarda o primeiro token de uma sessão (nova família).
	Create(ctx context.Context, rt RefreshToken) error

	// Rotate consome o token identificado por oldHash e grava next na mesma
	// família, atomicamente. next vem sem UserID/FamilyID/TokenVersion: o
	// repositório os copia do token antigo e devolve o token gravado.
	//   - hash desconhecido, revogado ou expirado: ErrInvalidRefreshToken
	//   - token já usado: revoga a família e devolve ErrTokenReuse.
	Rotate(ctx context.Context, oldHash string, next RefreshToken, now time.Time) (RefreshToken, error)

	// RevokeFamily revoga toda a sessão do token e devolve o dono.
	// Hash desconhecido: ErrInvalidRefreshToken.
	RevokeFamily(ctx context.Context, hash string, now time.Time) (uuid.UUID, error)
}

// Users é o que o módulo precisa do módulo user; user.Service satisfaz.
type Users interface {
	FindByEmail(ctx context.Context, email string) (*user.User, error)
	Get(ctx context.Context, id uuid.UUID) (*user.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error
}

// AccessTokens emite access tokens.
type AccessTokens interface {
	Issue(userID uuid.UUID, roles []string) (string, error)
	TTL() time.Duration
}
