package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"users-api/internal/auth"
	"users-api/internal/auth/authtest"
	"users-api/internal/plataform/password"
	"users-api/internal/plataform/token"
	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	goodPass   = "Senha@123"
)

var errBoom = errors.New("boom")

type fixture struct {
	svc    *auth.Service
	users  *user.Service
	repo   *authtest.FakeRepository
	tokens *token.Manager
	hasher *password.Hasher
	user   *user.User
}

func newFixture(t *testing.T, refreshTTL time.Duration) fixture {
	t.Helper()
	hasher, err := password.NewHasher(testSecret)
	require.NoError(t, err)
	tokens, err := token.NewManager(testSecret, 15*time.Minute)
	require.NoError(t, err)

	users := user.NewService(usertest.NewFakeRepository(), hasher)
	repo := authtest.NewFakeRepository()
	u, err := users.Create(context.Background(), user.Input{Name: "Ana", Email: "ana@example.com", Password: goodPass, Age: 30})
	require.NoError(t, err)

	return fixture{
		svc: auth.NewService(repo, users, hasher, tokens, refreshTTL), users: users,
		repo: repo, tokens: tokens, hasher: hasher, user: u,
	}
}

func (f fixture) tokenVersion(t *testing.T) int {
	t.Helper()
	u, err := f.users.Get(context.Background(), f.user.ID)
	require.NoError(t, err)
	return u.TokenVersion
}

func TestService_Login(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()

	pair, err := f.svc.Login(ctx, "ANA@example.com", goodPass)

	require.NoError(t, err)
	claims, err := f.tokens.Parse(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, f.user.ID, claims.UserID)
	assert.Equal(t, []string{"user"}, claims.Roles)
	assert.Equal(t, 15*time.Minute, pair.ExpiresIn)
	assert.NotEmpty(t, pair.RefreshToken)
	assert.Equal(t, 1, f.repo.Count())
}

func TestService_Login_Failures(t *testing.T) {
	f := newFixture(t, time.Hour)

	tests := map[string][2]string{
		"senha errada":      {"ana@example.com", "Outra@123"},
		"email inexistente": {"ninguem@example.com", goodPass},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			pair, err := f.svc.Login(context.Background(), c[0], c[1])
			assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
			assert.Nil(t, pair)
		})
	}
	assert.Zero(t, f.repo.Count(), "falhas não criam sessão")
}

func TestService_Login_StoresOnlyTheHashOfTheRefreshToken(t *testing.T) {
	f := newFixture(t, time.Hour)
	repo := new(authtest.MockRepository)
	var stored auth.RefreshToken
	repo.On("Create", mock.Anything, mock.AnythingOfType("auth.RefreshToken")).
		Run(func(args mock.Arguments) { stored = args.Get(1).(auth.RefreshToken) }).
		Return(nil)
	svc := auth.NewService(repo, f.users, f.hasher, f.tokens, time.Hour)

	pair, err := svc.Login(context.Background(), "ana@example.com", goodPass)

	require.NoError(t, err)
	assert.NotEqual(t, pair.RefreshToken, stored.TokenHash)
	assert.Len(t, stored.TokenHash, 64, "sha256 em hex")
	assert.Equal(t, f.user.ID, stored.UserID)
	assert.NotEqual(t, uuid.Nil(), stored.FamilyID)
	assert.True(t, stored.ExpiresAt.After(time.Now().Add(59*time.Minute)))
	repo.AssertExpectations(t)
}

func TestService_Login_RepoError(t *testing.T) {
	f := newFixture(t, time.Hour)
	repo := new(authtest.MockRepository)
	repo.On("Create", mock.Anything, mock.Anything).Return(errBoom)

	_, err := auth.NewService(repo, f.users, f.hasher, f.tokens, time.Hour).Login(context.Background(), "ana@example.com", goodPass)

	assert.ErrorIs(t, err, errBoom)
}

func TestService_Refresh_Rotates(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	first, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)

	second, err := f.svc.Refresh(ctx, first.RefreshToken)

	require.NoError(t, err)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)
	claims, err := f.tokens.Parse(second.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, f.user.ID, claims.UserID)

	third, err := f.svc.Refresh(ctx, second.RefreshToken)
	require.NoError(t, err, "o token novo funciona e continua a cadeia")
	assert.NotEqual(t, second.RefreshToken, third.RefreshToken)
}

func TestService_Refresh_TrazRolesAtuaisDoBanco(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	first, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)
	_, err = f.users.SetRoles(ctx, f.user.ID, []string{"user", "admin"})
	require.NoError(t, err)

	second, err := f.svc.Refresh(ctx, first.RefreshToken)

	require.NoError(t, err)
	claims, err := f.tokens.Parse(second.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, []string{"admin", "user"}, claims.Roles)
}

func TestService_Refresh_ReuseRevokesFamily(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	first, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)
	second, err := f.svc.Refresh(ctx, first.RefreshToken)
	require.NoError(t, err)

	_, err = f.svc.Refresh(ctx, first.RefreshToken) // reapresenta o token já usado

	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
	_, err = f.svc.Refresh(ctx, second.RefreshToken)
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken, "o token legítimo da mesma sessão também morre")
}

func TestService_Refresh_UsuarioExcluido(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	pair, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)
	require.NoError(t, f.users.Delete(ctx, f.user.ID))

	_, err = f.svc.Refresh(ctx, pair.RefreshToken)

	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
}

func TestService_Refresh_VersaoDivergenteRevogaASessao(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	pair, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)
	require.NoError(t, f.users.UpdatePassword(ctx, f.user.ID, "novo-hash"))

	_, err = f.svc.Refresh(ctx, pair.RefreshToken)

	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
	assert.Equal(t, 1, f.tokenVersion(t))
}

func TestService_Refresh_Invalid(t *testing.T) {
	t.Run("desconhecido", func(t *testing.T) {
		f := newFixture(t, time.Hour)
		_, err := f.svc.Refresh(context.Background(), "nao-existe")
		assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
	})
	t.Run("expirado", func(t *testing.T) {
		f := newFixture(t, -time.Second)
		pair, err := f.svc.Login(context.Background(), "ana@example.com", goodPass)
		require.NoError(t, err)

		_, err = f.svc.Refresh(context.Background(), pair.RefreshToken)
		assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
	})
	t.Run("erro do repositório", func(t *testing.T) {
		f := newFixture(t, time.Hour)
		repo := new(authtest.MockRepository)
		repo.On("Rotate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(auth.RefreshToken{}, errBoom)

		_, err := auth.NewService(repo, f.users, f.hasher, f.tokens, time.Hour).Refresh(context.Background(), "x")
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestService_Logout(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	pair, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)
	refreshed, err := f.svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	other, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)

	require.NoError(t, f.svc.Logout(ctx, refreshed.RefreshToken))

	_, err = f.svc.Refresh(ctx, refreshed.RefreshToken)
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
	_, err = f.svc.Refresh(ctx, other.RefreshToken)
	assert.NoError(t, err, "logout encerra só a sessão do token")
}

func TestService_Logout_UnknownTokenIsNoop(t *testing.T) {
	f := newFixture(t, time.Hour)

	assert.NoError(t, f.svc.Logout(context.Background(), "nao-existe"))
}

func TestService_ChangePassword(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	pair, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)

	require.NoError(t, f.svc.ChangePassword(ctx, f.user.ID, goodPass, "Nova@4567"))

	_, err = f.svc.Login(ctx, "ana@example.com", goodPass)
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials, "senha antiga não vale mais")
	_, err = f.svc.Login(ctx, "ana@example.com", "Nova@4567")
	assert.NoError(t, err)
	_, err = f.svc.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken, "sessões antigas são revogadas")
	assert.Equal(t, 1, f.tokenVersion(t))
}

func TestService_ChangePassword_Failures(t *testing.T) {
	f := newFixture(t, time.Hour)
	ctx := context.Background()
	pair, err := f.svc.Login(ctx, "ana@example.com", goodPass)
	require.NoError(t, err)

	t.Run("senha atual errada não altera nada", func(t *testing.T) {
		err := f.svc.ChangePassword(ctx, f.user.ID, "Errada@123", "Nova@4567")

		assert.ErrorIs(t, err, auth.ErrWrongPassword)
		assert.Equal(t, 0, f.tokenVersion(t))
		_, err = f.svc.Refresh(ctx, pair.RefreshToken)
		assert.NoError(t, err, "sessão continua válida")
		_, err = f.svc.Login(ctx, "ana@example.com", goodPass)
		assert.NoError(t, err)
	})
	t.Run("usuário inexistente", func(t *testing.T) {
		err := f.svc.ChangePassword(ctx, uuid.NewV7(), goodPass, "Nova@4567")
		assert.ErrorIs(t, err, user.ErrNotFound)
	})
}
