package user

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"
	"uuid"

	"users-api/internal/plataform/password"
)

type Input struct {
	ID       uuid.UUID
	Name     string
	Password string
	Email    string
	Age      int
}

type Service struct {
	repo   Repository
	hasher *password.Hasher
}

func NewService(repository Repository, hasher *password.Hasher) *Service {
	return &Service{repo: repository, hasher: hasher}
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	if id == uuid.Nil() {
		return nil, ErrInvalidID
	}

	user, err := s.repo.FindOne(ctx, id)

	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) Create(ctx context.Context, input Input) (*User, error) {
	hash, err := s.hasher.Hash(input.Password)

	if err != nil {
		return nil, err
	}

	user := User{Name: input.Name, Email: input.Email, Age: input.Age}

	user.Password = hash
	user.ID = uuid.NewV7()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	userResponse, err := s.repo.Save(ctx, user)

	if err != nil {
		return nil, err
	}

	return userResponse, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input Input) (*User, error) {
	user, err := s.Get(ctx, id)

	if err != nil {
		return nil, err
	}

	user.Name = input.Name
	user.Email = input.Email
	user.Age = input.Age

	userResponse, err := s.repo.Update(ctx, *user)

	if err != nil {
		return nil, err
	}

	return userResponse, nil
}

func (s *Service) GetAll(ctx context.Context) ([]*User, error) {
	users, err := s.repo.FindAll(ctx)

	if err != nil {
		return nil, err
	}

	return users, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.repo.Delete(ctx, id)

	if err != nil {
		return err
	}

	return nil
}

// roleNamePattern espelha o CHECK da tabela roles.
var roleNamePattern = regexp.MustCompile(`^[a-z0-9_]{2,32}$`)

func validRoleName(name string) bool { return roleNamePattern.MatchString(name) }

// SetRoles substitui os roles do usuário. Roles novos são criados na hora.
func (s *Service) SetRoles(ctx context.Context, id uuid.UUID, roles []string) (*User, error) {
	if id == uuid.Nil() {
		return nil, ErrInvalidID
	}

	unique := make([]string, 0, len(roles))
	for _, role := range roles {
		if !validRoleName(role) {
			return nil, ErrInvalidRole
		}
		if !slices.Contains(unique, role) {
			unique = append(unique, role)
		}
	}
	if len(unique) == 0 {
		return nil, ErrInvalidRole
	}

	return s.repo.SetRoles(ctx, id, unique)
}

func (s *Service) FindByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.FindByEmail(ctx, email)
}

// UpdatePassword grava um hash já calculado (quem chama usa o Hasher).
func (s *Service) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	return s.repo.UpdatePassword(ctx, id, hash)
}

// EnsureAdmin garante que exista um admin com o e-mail informado. É idempotente:
// se o usuário já existe só ganha o role admin, sem trocar a senha.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) error {
	existing, err := s.repo.FindByEmail(ctx, email)

	if errors.Is(err, ErrNotFound) {
		_, err = s.Create(ctx, Input{Name: "Admin", Email: email, Password: password, Age: 1})
		if errors.Is(err, ErrEmailTaken) {
			// Outra instância criou entre a busca e o insert.
			existing, err = s.repo.FindByEmail(ctx, email)
		} else if err != nil {
			return err
		} else {
			existing, err = s.repo.FindByEmail(ctx, email)
		}
	}
	if err != nil {
		return err
	}

	if slices.Contains(existing.Roles, RoleAdmin) {
		return nil
	}

	_, err = s.repo.SetRoles(ctx, existing.ID, append(slices.Clone(existing.Roles), RoleAdmin))
	return err
}
