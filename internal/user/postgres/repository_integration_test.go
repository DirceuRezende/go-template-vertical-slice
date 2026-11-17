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
	_, err := testPool.Exec(context.Background(), "TRUNCATE users CASCADE")
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

func saveUser(t *testing.T, repo *postgres.Repository, name string) *user.User {
	t.Helper()
	saved, err := repo.Save(context.Background(), newUser(name, time.Now().UTC()))
	require.NoError(t, err)
	return saved
}

func TestRepository_Save_AssignsUserRole(t *testing.T) {
	repo := newRepo(t)

	saved := saveUser(t, repo, "ana")

	assert.Equal(t, []string{"user"}, saved.Roles)
	got, err := repo.FindOne(context.Background(), saved.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"user"}, got.Roles)
	assert.Equal(t, 0, got.TokenVersion)
}

func TestRepository_FindByEmail(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	saved := saveUser(t, repo, "ana")

	got, err := repo.FindByEmail(ctx, "ANA@Example.com")
	require.NoError(t, err)
	assert.Equal(t, saved.ID, got.ID)
	assert.Equal(t, "hash", got.Password)

	_, err = repo.FindByEmail(ctx, "ninguem@example.com")
	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestRepository_FindAll_LoadsRoles(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := saveUser(t, repo, "ana")
	saveUser(t, repo, "bia")
	_, err := repo.SetRoles(ctx, a.ID, []string{"user", "admin"})
	require.NoError(t, err)

	all, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	byName := map[string][]string{all[0].Name: all[0].Roles, all[1].Name: all[1].Roles}
	assert.Equal(t, []string{"admin", "user"}, byName["ana"])
	assert.Equal(t, []string{"user"}, byName["bia"])
}

func TestRepository_SetRoles_ReplacesAndCreates(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	saved := saveUser(t, repo, "ana")

	got, err := repo.SetRoles(ctx, saved.ID, []string{"editor", "user"})
	require.NoError(t, err)
	assert.Equal(t, []string{"editor", "user"}, got.Roles)
	assert.Equal(t, saved.TokenVersion, got.TokenVersion, "roles não mexem em token_version")

	var exists bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE name = 'editor')`).Scan(&exists))
	assert.True(t, exists, "role novo deve ser criado")

	got, err = repo.SetRoles(ctx, saved.ID, []string{"user"})
	require.NoError(t, err)
	assert.Equal(t, []string{"user"}, got.Roles)
}

func TestRepository_SetRoles_NotFound(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.SetRoles(context.Background(), uuid.NewV7(), []string{"user"})

	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestRepository_SetRoles_LastAdmin(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := saveUser(t, repo, "ana")
	b := saveUser(t, repo, "bia")
	_, err := repo.SetRoles(ctx, a.ID, []string{"user", "admin"})
	require.NoError(t, err)

	_, err = repo.SetRoles(ctx, a.ID, []string{"user"})
	assert.ErrorIs(t, err, user.ErrLastAdmin)

	got, err := repo.FindOne(ctx, a.ID)
	require.NoError(t, err)
	assert.Contains(t, got.Roles, "admin", "falha não pode alterar nada")

	_, err = repo.SetRoles(ctx, b.ID, []string{"user", "admin"})
	require.NoError(t, err)
	_, err = repo.SetRoles(ctx, a.ID, []string{"user"})
	assert.NoError(t, err)
}

func TestRepository_Delete_LastAdmin(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := saveUser(t, repo, "ana")
	b := saveUser(t, repo, "bia")
	_, err := repo.SetRoles(ctx, a.ID, []string{"user", "admin"})
	require.NoError(t, err)

	assert.ErrorIs(t, repo.Delete(ctx, a.ID), user.ErrLastAdmin)
	assert.NoError(t, repo.Delete(ctx, b.ID), "usuário comum pode ser excluído")
	assert.ErrorIs(t, repo.Delete(ctx, uuid.NewV7()), user.ErrNotFound)

	var links int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE user_id = $1`, b.ID.String()).Scan(&links))
	assert.Zero(t, links, "vínculos de roles saem em cascata")
}

func TestRepository_Delete_ConcurrentAdminsKeepsOne(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	a := saveUser(t, repo, "ana")
	b := saveUser(t, repo, "bia")
	for _, u := range []*user.User{a, b} {
		_, err := repo.SetRoles(ctx, u.ID, []string{"user", "admin"})
		require.NoError(t, err)
	}

	results := make(chan error, 2)
	for _, id := range []uuid.UUID{a.ID, b.ID} {
		go func() { results <- repo.Delete(ctx, id) }()
	}
	errs := []error{<-results, <-results}

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		} else {
			assert.ErrorIs(t, err, user.ErrLastAdmin)
		}
	}
	assert.Equal(t, 1, succeeded, "exatamente uma exclusão pode vencer")

	var admins int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE role = 'admin'`).Scan(&admins))
	assert.Equal(t, 1, admins)
}

func TestRepository_UpdatePassword(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	saved := saveUser(t, repo, "ana")

	require.NoError(t, repo.UpdatePassword(ctx, saved.ID, "novo-hash"))

	got, err := repo.FindOne(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "novo-hash", got.Password)
	assert.Equal(t, saved.TokenVersion+1, got.TokenVersion)

	assert.ErrorIs(t, repo.UpdatePassword(ctx, uuid.NewV7(), "h"), user.ErrNotFound)
}

func TestRepository_Update_KeepsRolesAndPassword(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	saved := saveUser(t, repo, "ana")
	_, err := repo.SetRoles(ctx, saved.ID, []string{"user", "admin"})
	require.NoError(t, err)

	saved.Name = "Ana Maria"
	updated, err := repo.Update(ctx, *saved)

	require.NoError(t, err)
	assert.Equal(t, "Ana Maria", updated.Name)
	assert.Equal(t, []string{"admin", "user"}, updated.Roles)
	assert.Equal(t, "hash", updated.Password)
}
