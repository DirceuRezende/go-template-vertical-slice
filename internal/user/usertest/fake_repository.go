// Package usertest reúne doubles de teste para o módulo user.
package usertest

import (
	"context"
	"sort"
	"strings"
	"sync"
	"uuid"

	"users-api/internal/user"
)

// FakeRepository é uma implementação em memória de user.Repository.
type FakeRepository struct {
	mu    sync.Mutex
	users map[uuid.UUID]user.User
}

var _ user.Repository = (*FakeRepository)(nil)

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{users: map[uuid.UUID]user.User{}}
}

func (f *FakeRepository) Save(_ context.Context, u user.User) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.emailTaken(u.Email, u.ID) {
		return nil, user.ErrEmailTaken
	}
	f.users[u.ID] = u
	return &u, nil
}

func (f *FakeRepository) Update(_ context.Context, u user.User) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.users[u.ID]
	if !ok {
		return nil, user.ErrNotFound
	}
	if f.emailTaken(u.Email, u.ID) {
		return nil, user.ErrEmailTaken
	}
	cur.Name, cur.Email, cur.Age = u.Name, u.Email, u.Age
	f.users[u.ID] = cur
	return &cur, nil
}

func (f *FakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[id]; !ok {
		return user.ErrNotFound
	}
	delete(f.users, id)
	return nil
}

func (f *FakeRepository) FindOne(_ context.Context, id uuid.UUID) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return &u, nil
}

func (f *FakeRepository) FindAll(_ context.Context) ([]*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*user.User, 0, len(f.users))
	for _, u := range f.users {
		u := u
		out = append(out, &u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// emailTaken espelha o índice único case-insensitive do Postgres. Exige o lock.
func (f *FakeRepository) emailTaken(email string, except uuid.UUID) bool {
	for id, u := range f.users {
		if id != except && strings.EqualFold(u.Email, email) {
			return true
		}
	}
	return false
}
