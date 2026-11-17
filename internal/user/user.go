package user

import (
	"context"
	"errors"
	"time"
	"uuid"
)

var (
	ErrNotFound   = errors.New("Usuário não encontrado")
	ErrInvalidID  = errors.New("ID do usuário inválido")
	ErrEmailTaken = errors.New("Email já cadastrado")
)

type User struct {
	ID        uuid.UUID
	Name      string
	Email     string
	Password  string
	Age       int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repository interface {
	Save(ctx context.Context, u User) (*User, error)
	Update(ctx context.Context, u User) (*User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	FindOne(ctx context.Context, id uuid.UUID) (*User, error)
	FindAll(ctx context.Context) ([]*User, error)
}
