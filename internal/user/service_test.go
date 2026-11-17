package user_test

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"users-api/internal/plataform/password"
	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testPepper = "0123456789abcdef0123456789abcdef"

var errBoom = errors.New("boom")

func newHasher(t *testing.T) *password.Hasher {
	t.Helper()
	h, err := password.NewHasher(testPepper)
	require.NoError(t, err)
	return h
}

func TestService_Create_HashesPassword(t *testing.T) {
	hasher := newHasher(t)
	repo := new(usertest.MockRepository)
	svc := user.NewService(repo, hasher)

	var saved user.User
	repo.On("Save", mock.Anything, mock.AnythingOfType("user.User")).
		Run(func(args mock.Arguments) { saved = args.Get(1).(user.User) }).
		Return(&user.User{Name: "Ana"}, nil)

	got, err := svc.Create(context.Background(), user.Input{Name: "Ana", Email: "a@a.com", Password: "Senha@123", Age: 30})

	require.NoError(t, err)
	assert.Equal(t, "Ana", got.Name)
	assert.NotEqual(t, uuid.Nil(), saved.ID)
	assert.NotEqual(t, "Senha@123", saved.Password)
	assert.True(t, hasher.Matches(saved.Password, "Senha@123"))
	assert.Equal(t, "a@a.com", saved.Email)
	assert.Equal(t, 30, saved.Age)
	repo.AssertExpectations(t)
}

func TestService_Create_RepoError(t *testing.T) {
	repo := new(usertest.MockRepository)
	repo.On("Save", mock.Anything, mock.Anything).Return(nil, errBoom)

	got, err := user.NewService(repo, newHasher(t)).Create(context.Background(), user.Input{Password: "Senha@123"})

	assert.ErrorIs(t, err, errBoom)
	assert.Nil(t, got)
}

func TestService_Get(t *testing.T) {
	id := uuid.New()

	t.Run("id nil não chama o repositório", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		got, err := user.NewService(repo, newHasher(t)).Get(context.Background(), uuid.Nil())

		assert.ErrorIs(t, err, user.ErrInvalidID)
		assert.Nil(t, got)
		repo.AssertNotCalled(t, "FindOne", mock.Anything, mock.Anything)
	})

	t.Run("sucesso", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindOne", mock.Anything, id).Return(&user.User{ID: id, Name: "Ana"}, nil)

		got, err := user.NewService(repo, newHasher(t)).Get(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, id, got.ID)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindOne", mock.Anything, id).Return(nil, user.ErrNotFound)

		_, err := user.NewService(repo, newHasher(t)).Get(context.Background(), id)

		assert.ErrorIs(t, err, user.ErrNotFound)
	})
}

func TestService_Update(t *testing.T) {
	id := uuid.New()

	t.Run("atualiza campos e preserva a senha", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindOne", mock.Anything, id).Return(&user.User{ID: id, Name: "Ana", Password: "hash"}, nil)
		repo.On("Update", mock.Anything, mock.MatchedBy(func(u user.User) bool {
			return u.ID == id && u.Name == "Bia" && u.Email == "b@b.com" && u.Age == 40 && u.Password == "hash"
		})).Return(&user.User{ID: id, Name: "Bia"}, nil)

		got, err := user.NewService(repo, newHasher(t)).Update(context.Background(), id, user.Input{Name: "Bia", Email: "b@b.com", Age: 40})

		require.NoError(t, err)
		assert.Equal(t, "Bia", got.Name)
		repo.AssertExpectations(t)
	})

	t.Run("erro no FindOne interrompe antes do Update", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindOne", mock.Anything, id).Return(nil, user.ErrNotFound)

		_, err := user.NewService(repo, newHasher(t)).Update(context.Background(), id, user.Input{})

		assert.ErrorIs(t, err, user.ErrNotFound)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("erro no Update é propagado", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindOne", mock.Anything, id).Return(&user.User{ID: id}, nil)
		repo.On("Update", mock.Anything, mock.Anything).Return(nil, errBoom)

		_, err := user.NewService(repo, newHasher(t)).Update(context.Background(), id, user.Input{})

		assert.ErrorIs(t, err, errBoom)
	})
}

func TestService_GetAll(t *testing.T) {
	t.Run("sucesso", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindAll", mock.Anything).Return([]*user.User{{Name: "A"}, {Name: "B"}}, nil)

		got, err := user.NewService(repo, newHasher(t)).GetAll(context.Background())

		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("erro", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindAll", mock.Anything).Return(nil, errBoom)

		_, err := user.NewService(repo, newHasher(t)).GetAll(context.Background())

		assert.ErrorIs(t, err, errBoom)
	})
}

func TestService_Delete(t *testing.T) {
	id := uuid.New()

	repo := new(usertest.MockRepository)
	repo.On("Delete", mock.Anything, id).Return(nil).Once()
	repo.On("Delete", mock.Anything, id).Return(user.ErrNotFound).Once()
	svc := user.NewService(repo, newHasher(t))

	assert.NoError(t, svc.Delete(context.Background(), id))
	assert.ErrorIs(t, svc.Delete(context.Background(), id), user.ErrNotFound)
	repo.AssertExpectations(t)
}

func TestService_Lifecycle_WithFake(t *testing.T) {
	ctx := context.Background()
	svc := user.NewService(usertest.NewFakeRepository(), newHasher(t))

	created, err := svc.Create(ctx, user.Input{Name: "Ana", Email: "a@a.com", Password: "Senha@123", Age: 30})
	require.NoError(t, err)

	got, err := svc.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ana", got.Name)

	updated, err := svc.Update(ctx, created.ID, user.Input{Name: "Bia", Email: "b@b.com", Age: 31})
	require.NoError(t, err)
	assert.Equal(t, "Bia", updated.Name)
	assert.Equal(t, 31, updated.Age)

	all, err := svc.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, svc.Delete(ctx, created.ID))
	_, err = svc.Get(ctx, created.ID)
	assert.ErrorIs(t, err, user.ErrNotFound)
}

func TestService_Create_EmailTaken(t *testing.T) {
	repo := new(usertest.MockRepository)
	repo.On("Save", mock.Anything, mock.Anything).Return(nil, user.ErrEmailTaken)

	_, err := user.NewService(repo, newHasher(t)).Create(context.Background(), user.Input{Password: "Senha@123"})

	assert.ErrorIs(t, err, user.ErrEmailTaken)
}

func TestService_Update_EmailTaken(t *testing.T) {
	id := uuid.New()
	repo := new(usertest.MockRepository)
	repo.On("FindOne", mock.Anything, id).Return(&user.User{ID: id}, nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil, user.ErrEmailTaken)

	_, err := user.NewService(repo, newHasher(t)).Update(context.Background(), id, user.Input{Email: "x@x.com"})

	assert.ErrorIs(t, err, user.ErrEmailTaken)
}

func TestService_Create_DuplicateEmail_WithFake(t *testing.T) {
	ctx := context.Background()
	svc := user.NewService(usertest.NewFakeRepository(), newHasher(t))

	_, err := svc.Create(ctx, user.Input{Name: "Ana", Email: "ana@example.com", Password: "Senha@123", Age: 30})
	require.NoError(t, err)

	_, err = svc.Create(ctx, user.Input{Name: "Outra", Email: "ANA@example.com", Password: "Senha@123", Age: 31})
	assert.ErrorIs(t, err, user.ErrEmailTaken)
}

func TestService_SetRoles_DeduplicatesAndDelegates(t *testing.T) {
	id := uuid.New()
	repo := new(usertest.MockRepository)
	repo.On("SetRoles", mock.Anything, id, []string{"user", "editor"}).Return(&user.User{ID: id}, nil)

	_, err := user.NewService(repo, newHasher(t)).SetRoles(context.Background(), id, []string{"user", "editor", "user"})

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestService_SetRoles_Invalid(t *testing.T) {
	svc := user.NewService(new(usertest.MockRepository), newHasher(t))
	id := uuid.New()

	tests := map[string][]string{
		"vazio":          {},
		"maiúscula":      {"Admin"},
		"curto":          {"a"},
		"muito longo":    {"abcdefghijklmnopqrstuvwxyzabcdefg"},
		"caractere ruim": {"ad-min"},
	}
	for name, roles := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.SetRoles(context.Background(), id, roles)
			assert.ErrorIs(t, err, user.ErrInvalidRole)
		})
	}

	_, err := svc.SetRoles(context.Background(), uuid.Nil(), []string{"user"})
	assert.ErrorIs(t, err, user.ErrInvalidID)
}

func TestService_LastAdmin_WithFake(t *testing.T) {
	ctx := context.Background()
	svc := user.NewService(usertest.NewFakeRepository(), newHasher(t))
	a, err := svc.Create(ctx, user.Input{Name: "Ana", Email: "a@a.com", Password: "Senha@123", Age: 30})
	require.NoError(t, err)
	b, err := svc.Create(ctx, user.Input{Name: "Bia", Email: "b@b.com", Password: "Senha@123", Age: 30})
	require.NoError(t, err)

	_, err = svc.SetRoles(ctx, a.ID, []string{"user", "admin"})
	require.NoError(t, err)

	t.Run("único admin não perde o role nem é excluído", func(t *testing.T) {
		_, err := svc.SetRoles(ctx, a.ID, []string{"user"})
		assert.ErrorIs(t, err, user.ErrLastAdmin)
		assert.ErrorIs(t, svc.Delete(ctx, a.ID), user.ErrLastAdmin)
	})
	t.Run("com dois admins um pode sair", func(t *testing.T) {
		_, err := svc.SetRoles(ctx, b.ID, []string{"user", "admin"})
		require.NoError(t, err)

		_, err = svc.SetRoles(ctx, a.ID, []string{"user"})
		require.NoError(t, err)
		assert.ErrorIs(t, svc.Delete(ctx, b.ID), user.ErrLastAdmin)
	})
	t.Run("usuário comum pode ser excluído", func(t *testing.T) {
		assert.NoError(t, svc.Delete(ctx, a.ID))
	})
}

func TestService_EnsureAdmin(t *testing.T) {
	ctx := context.Background()
	hasher := newHasher(t)

	t.Run("cria o admin quando não existe", func(t *testing.T) {
		svc := user.NewService(usertest.NewFakeRepository(), hasher)

		require.NoError(t, svc.EnsureAdmin(ctx, "root@example.com", "Senha@123"))

		got, err := svc.FindByEmail(ctx, "ROOT@example.com")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"user", "admin"}, got.Roles)
		assert.True(t, hasher.Matches(got.Password, "Senha@123"))
	})
	t.Run("é idempotente", func(t *testing.T) {
		svc := user.NewService(usertest.NewFakeRepository(), hasher)
		require.NoError(t, svc.EnsureAdmin(ctx, "root@example.com", "Senha@123"))
		first, _ := svc.FindByEmail(ctx, "root@example.com")

		require.NoError(t, svc.EnsureAdmin(ctx, "root@example.com", "Outra@456"))

		all, err := svc.GetAll(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
		assert.Equal(t, first.Password, all[0].Password, "não deve trocar a senha")
	})
	t.Run("promove usuário existente sem mexer na senha", func(t *testing.T) {
		svc := user.NewService(usertest.NewFakeRepository(), hasher)
		created, err := svc.Create(ctx, user.Input{Name: "Ana", Email: "ana@example.com", Password: "Senha@123", Age: 30})
		require.NoError(t, err)

		require.NoError(t, svc.EnsureAdmin(ctx, "ana@example.com", "Outra@456"))

		got, err := svc.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"user", "admin"}, got.Roles)
		assert.True(t, hasher.Matches(got.Password, "Senha@123"))
	})
	t.Run("propaga erro do repositório", func(t *testing.T) {
		repo := new(usertest.MockRepository)
		repo.On("FindByEmail", mock.Anything, "x@x.com").Return(nil, errBoom)

		assert.ErrorIs(t, user.NewService(repo, hasher).EnsureAdmin(ctx, "x@x.com", "Senha@123"), errBoom)
	})
}

func TestService_UpdatePassword_IncrementaTokenVersion(t *testing.T) {
	ctx := context.Background()
	svc := user.NewService(usertest.NewFakeRepository(), newHasher(t))
	u, err := svc.Create(ctx, user.Input{Name: "Ana", Email: "a@a.com", Password: "Senha@123", Age: 30})
	require.NoError(t, err)
	assert.Equal(t, 0, u.TokenVersion)

	require.NoError(t, svc.UpdatePassword(ctx, u.ID, "novo-hash"))

	got, err := svc.Get(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.TokenVersion)
	assert.Equal(t, "novo-hash", got.Password)
	assert.ErrorIs(t, svc.UpdatePassword(ctx, uuid.New(), "h"), user.ErrNotFound)
}
