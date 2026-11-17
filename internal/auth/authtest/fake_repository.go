// Package authtest reúne doubles de teste para o módulo auth.
package authtest

import (
	"context"
	"sync"
	"time"
	"uuid"

	"users-api/internal/auth"
)

// FakeRepository é uma implementação em memória de auth.Repository, com as
// mesmas regras do Postgres (rotação de uso único, revogação de família).
type FakeRepository struct {
	mu     sync.Mutex
	tokens map[string]*auth.RefreshToken // por token_hash
}

var _ auth.Repository = (*FakeRepository)(nil)

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{tokens: map[string]*auth.RefreshToken{}}
}

func (f *FakeRepository) Create(_ context.Context, rt auth.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[rt.TokenHash] = &rt
	return nil
}

func (f *FakeRepository) Rotate(_ context.Context, oldHash string, next auth.RefreshToken, now time.Time) (auth.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	old, ok := f.tokens[oldHash]
	if !ok || old.RevokedAt != nil {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}
	if old.UsedAt != nil {
		f.revokeFamily(old.FamilyID, now)
		return auth.RefreshToken{UserID: old.UserID, FamilyID: old.FamilyID}, auth.ErrTokenReuse
	}
	if !old.ExpiresAt.After(now) {
		return auth.RefreshToken{}, auth.ErrInvalidRefreshToken
	}

	old.UsedAt = &now
	next.UserID, next.FamilyID, next.TokenVersion = old.UserID, old.FamilyID, old.TokenVersion
	f.tokens[next.TokenHash] = &next
	return next, nil
}

func (f *FakeRepository) RevokeFamily(_ context.Context, hash string, now time.Time) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tokens[hash]
	if !ok {
		return uuid.Nil(), auth.ErrInvalidRefreshToken
	}
	f.revokeFamily(t.FamilyID, now)
	return t.UserID, nil
}

// revokeFamily exige o lock.
func (f *FakeRepository) revokeFamily(family uuid.UUID, now time.Time) {
	for _, t := range f.tokens {
		if t.FamilyID == family && t.RevokedAt == nil {
			t.RevokedAt = &now
		}
	}
}

// Count devolve quantos tokens há (para asserts).
func (f *FakeRepository) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tokens)
}
