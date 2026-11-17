package user

import (
	"context"
	"errors"
	"time"
	"uuid"

	"users-api/internal/plataform/authn"
)

var (
	ErrNotFound    = errors.New("Usuário não encontrado")
	ErrInvalidID   = errors.New("ID do usuário inválido")
	ErrEmailTaken  = errors.New("Email já cadastrado")
	ErrForbidden   = errors.New("Acesso negado")
	ErrLastAdmin   = errors.New("Não é possível remover o último administrador")
	ErrInvalidRole = errors.New("Nome de role inválido")
)

// RoleUser é atribuído a todo cadastro; RoleAdmin concede acesso administrativo.
const (
	RoleUser  = "user"
	RoleAdmin = authn.AdminRole
)

type User struct {
	ID        uuid.UUID
	Name      string
	Email     string
	Password  string
	Age       int
	Roles     []string
	CreatedAt time.Time
	UpdatedAt time.Time

	// TokenVersion avança a cada troca de senha; refresh tokens emitidos com outra
	// versão deixam de valer.
	TokenVersion int
}

type Repository interface {
	Save(ctx context.Context, u User) (*User, error)
	Update(ctx context.Context, u User) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	FindOne(ctx context.Context, id uuid.UUID) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)

	// FindByEmail busca ignorando maiúsculas/minúsculas, como o índice único.
	FindByEmail(ctx context.Context, email string) (*User, error)
	// UpdatePassword grava o novo hash e incrementa TokenVersion.
	UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error
	// SetRoles substitui os roles (criando os inexistentes). Devolve ErrLastAdmin
	// se removeria o último admin.
	SetRoles(ctx context.Context, id uuid.UUID, roles []string) (*User, error)
}
