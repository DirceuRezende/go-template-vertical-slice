//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"
	"uuid"

	"users-api/internal/plataform/database"
	"users-api/internal/testutil"
	"users-api/internal/user"
	"users-api/internal/user/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	url, terminate, err := testutil.StartPostgres(ctx)
	if err != nil {
		panic(err)
	}
	code := func() int {
		defer terminate()
		if err := database.MigrateUp(url); err != nil {
			panic(err)
		}
		testPool, err = database.NewPool(ctx, database.DefaultConfig(url, "user-it"))
		if err != nil {
			panic(err)
		}
		defer testPool.Close()
		return m.Run()
	}()
	os.Exit(code)
}

func newRepo(t *testing.T) *postgres.Repository {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "TRUNCATE users")
	require.NoError(t, err)
	return postgres.NewRepository(testPool)
}

func newUser(name string, createdAt time.Time) user.User {
	return user.User{
		ID: uuid.NewV7(), Name: name, Email: name + "@example.com",
		Password: "hash", Age: 30, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func TestRepository_SaveAndFindOne(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u := newUser("ana", time.Now().UTC().Truncate(time.Microsecond))

	saved, err := repo.Save(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, u.ID, saved.ID)

	got, err := repo.FindOne(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "ana", got.Name)
	assert.Equal(t, "ana@example.com", got.Email)
	assert.Equal(t, "hash", got.Password)
	assert.Equal(t, 30, got.Age)
	assert.WithinDuration(t, u.CreatedAt, got.CreatedAt, time.Millisecond)
}

func TestRepository_FindOne_NotFound(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.FindOne(context.Background(), uuid.New())

	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestRepository_FindAll_OrderedByCreatedAt(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, u := range []user.User{
		newUser("terceiro", now.Add(2*time.Second)),
		newUser("primeiro", now),
		newUser("segundo", now.Add(time.Second)),
	} {
		_, err := repo.Save(ctx, u)
		require.NoError(t, err)
	}

	all, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []string{"primeiro", "segundo", "terceiro"}, []string{all[0].Name, all[1].Name, all[2].Name})
}

func TestRepository_FindAll_Empty(t *testing.T) {
	repo := newRepo(t)

	all, err := repo.FindAll(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, all)
	assert.Empty(t, all)
}

func TestRepository_Update(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u := newUser("ana", time.Now().UTC().Add(-time.Hour))
	_, err := repo.Save(ctx, u)
	require.NoError(t, err)

	u.Name, u.Email, u.Age = "bia", "bia@example.com", 41
	u.Password = "ignorada"
	updated, err := repo.Update(ctx, u)

	require.NoError(t, err)
	assert.Equal(t, "bia", updated.Name)
	assert.Equal(t, 41, updated.Age)
	assert.Equal(t, "hash", updated.Password, "Update não deve alterar a senha")
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt))
}

func TestRepository_Update_NotFound(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.Update(context.Background(), newUser("x", time.Now()))

	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestRepository_Delete(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u := newUser("ana", time.Now().UTC())
	_, err := repo.Save(ctx, u)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, u.ID))
	assert.ErrorIs(t, repo.Delete(ctx, u.ID), user.ErrNotFound)
	_, err = repo.FindOne(ctx, u.ID)
	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestRepository_Save_DuplicateID(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u := newUser("ana", time.Now().UTC())
	_, err := repo.Save(ctx, u)
	require.NoError(t, err)

	_, err = repo.Save(ctx, u)

	assert.Error(t, err)
}

func TestRepository_Save_DuplicateEmailRejected(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := newUser("ana", time.Now().UTC())
	_, err := repo.Save(ctx, a)
	require.NoError(t, err)

	b := newUser("outra", time.Now().UTC())
	b.Email = "ANA@Example.com" // case-insensitive
	_, err = repo.Save(ctx, b)

	assert.ErrorIs(t, err, user.ErrEmailTaken)
}

func TestRepository_Save_DuplicateIDIsNotEmailTaken(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := newUser("ana", time.Now().UTC())
	_, err := repo.Save(ctx, a)
	require.NoError(t, err)

	a.Email = "outro@example.com"
	_, err = repo.Save(ctx, a)

	require.Error(t, err)
	assert.NotErrorIs(t, err, user.ErrEmailTaken)
}

func TestRepository_Update_Email(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a, b := newUser("ana", time.Now().UTC()), newUser("bia", time.Now().UTC())
	for _, u := range []user.User{a, b} {
		_, err := repo.Save(ctx, u)
		require.NoError(t, err)
	}

	a.Email = b.Email
	_, err := repo.Update(ctx, a)
	assert.ErrorIs(t, err, user.ErrEmailTaken)

	b.Name = "bia 2" // mantém o próprio email
	_, err = repo.Update(ctx, b)
	assert.NoError(t, err)
}
