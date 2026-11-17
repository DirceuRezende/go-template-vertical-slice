package authtest

import (
	"context"
	"time"
	"uuid"

	"users-api/internal/auth"

	"github.com/stretchr/testify/mock"
)

// MockRepository é um mock testify de auth.Repository.
type MockRepository struct{ mock.Mock }

var _ auth.Repository = (*MockRepository)(nil)

func (m *MockRepository) Create(ctx context.Context, rt auth.RefreshToken) error {
	return m.Called(ctx, rt).Error(0)
}

func (m *MockRepository) Rotate(ctx context.Context, oldHash string, next auth.RefreshToken, now time.Time) (auth.RefreshToken, error) {
	args := m.Called(ctx, oldHash, next, now)
	r, _ := args.Get(0).(auth.RefreshToken)
	return r, args.Error(1)
}

func (m *MockRepository) RevokeFamily(ctx context.Context, hash string, now time.Time) (uuid.UUID, error) {
	args := m.Called(ctx, hash, now)
	id, _ := args.Get(0).(uuid.UUID)
	return id, args.Error(1)
}
