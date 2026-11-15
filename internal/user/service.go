package user

import (
	"context"
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
