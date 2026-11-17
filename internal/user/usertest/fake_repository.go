// Package usertest reúne doubles de teste para o módulo user.
package usertest

import (
	"context"
	"slices"
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
	u.Roles = []string{user.RoleUser}
	f.users[u.ID] = u
	return clone(u), nil
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
	return clone(cur), nil
}

func (f *FakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[id]; !ok {
		return user.ErrNotFound
	}
	if f.isLastAdmin(id) {
		return user.ErrLastAdmin
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
	return clone(u), nil
}

func (f *FakeRepository) FindAll(_ context.Context) ([]*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*user.User, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, clone(u))
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

func (f *FakeRepository) FindByEmail(_ context.Context, email string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return clone(u), nil
		}
	}
	return nil, user.ErrNotFound
}

func (f *FakeRepository) UpdatePassword(_ context.Context, id uuid.UUID, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return user.ErrNotFound
	}
	u.Password = hash
	u.TokenVersion++
	f.users[id] = u
	return nil
}

func (f *FakeRepository) SetRoles(_ context.Context, id uuid.UUID, roles []string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	if !slices.Contains(roles, user.RoleAdmin) && f.isLastAdmin(id) {
		return nil, user.ErrLastAdmin
	}
	sorted := slices.Clone(roles)
	slices.Sort(sorted)
	u.Roles = sorted
	f.users[id] = u
	return clone(u), nil
}

// isLastAdmin informa se id é o único admin existente. Exige o lock.
func (f *FakeRepository) isLastAdmin(id uuid.UUID) bool {
	admins := 0
	isTarget := false
	for uid, u := range f.users {
		if slices.Contains(u.Roles, user.RoleAdmin) {
			admins++
			isTarget = isTarget || uid == id
		}
	}
	return admins == 1 && isTarget
}

// clone devolve uma cópia que não compartilha o slice de roles com o armazenado.
func clone(u user.User) *user.User {
	u.Roles = slices.Clone(u.Roles)
	return &u
}
