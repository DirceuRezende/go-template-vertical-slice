package usertest

import (
	"context"
	"uuid"

	"users-api/internal/user"

	"github.com/stretchr/testify/mock"
)

// MockRepository é um mock testify de user.Repository.
type MockRepository struct{ mock.Mock }

var _ user.Repository = (*MockRepository)(nil)

func (m *MockRepository) Save(ctx context.Context, u user.User) (*user.User, error) {
	args := m.Called(ctx, u)
	r, _ := args.Get(0).(*user.User)
	return r, args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, u user.User) (*user.User, error) {
	args := m.Called(ctx, u)
	r, _ := args.Get(0).(*user.User)
	return r, args.Error(1)
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockRepository) FindOne(ctx context.Context, id uuid.UUID) (*user.User, error) {
	args := m.Called(ctx, id)
	r, _ := args.Get(0).(*user.User)
	return r, args.Error(1)
}

func (m *MockRepository) FindAll(ctx context.Context) ([]*user.User, error) {
	args := m.Called(ctx)
	r, _ := args.Get(0).([]*user.User)
	return r, args.Error(1)
}

func (m *MockRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	args := m.Called(ctx, email)
	r, _ := args.Get(0).(*user.User)
	return r, args.Error(1)
}

func (m *MockRepository) UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error {
	return m.Called(ctx, id, hash).Error(0)
}

func (m *MockRepository) SetRoles(ctx context.Context, id uuid.UUID, roles []string) (*user.User, error) {
	args := m.Called(ctx, id, roles)
	r, _ := args.Get(0).(*user.User)
	return r, args.Error(1)
}
