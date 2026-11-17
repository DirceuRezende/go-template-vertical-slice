//go:build integration

package postgres_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
	"uuid"

	"users-api/internal/auth"
	"users-api/internal/auth/postgres"
	"users-api/internal/plataform/database"
	"users-api/internal/testutil"
	"users-api/internal/user"
	userpg "users-api/internal/user/postgres"

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
		testPool, err = database.NewPool(ctx, database.DefaultConfig(url, "auth-it"))
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

func newUserID(t *testing.T, name string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	u, err := userpg.NewRepository(testPool).Save(context.Background(), user.User{
		ID: uuid.NewV7(), Name: name, Email: name + "@example.com", Password: "hash", Age: 30, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	return u.ID
}

func newToken(userID, family uuid.UUID, hash string, ttl time.Duration) auth.RefreshToken {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return auth.RefreshToken{
		ID: uuid.NewV7(), UserID: userID, FamilyID: family,
		TokenHash: hash, ExpiresAt: now.Add(ttl), CreatedAt: now,
	}
}

// next monta o token seguinte como o service faz: sem user/família.
func next(hash string) auth.RefreshToken {
	rt := newToken(uuid.Nil(), uuid.Nil(), hash, time.Hour)
	return rt
}

func revokedAt(t *testing.T, hash string) *time.Time {
	t.Helper()
	var at *time.Time
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1`, hash).Scan(&at))
	return at
}

func TestRepository_Rotate(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid, family := newUserID(t, "ana"), uuid.NewV7()
	require.NoError(t, repo.Create(ctx, newToken(uid, family, "h1", time.Hour)))

	got, err := repo.Rotate(ctx, "h1", next("h2"), time.Now())

	require.NoError(t, err)
	assert.Equal(t, uid, got.UserID)
	assert.Equal(t, family, got.FamilyID)
	assert.Equal(t, "h2", got.TokenHash)

	_, err = repo.Rotate(ctx, "h2", next("h3"), time.Now())
	assert.NoError(t, err, "a cadeia continua")
}

func TestRepository_Rotate_Invalid(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "expirado", -time.Minute)))
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "revogado", time.Hour)))
	_, err := repo.RevokeFamily(ctx, "revogado", time.Now())
	require.NoError(t, err)

	for _, hash := range []string{"desconhecido", "expirado", "revogado"} {
		_, err := repo.Rotate(ctx, hash, next("novo-"+hash), time.Now())
		assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken, hash)
	}

	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE token_hash LIKE 'novo-%'`).Scan(&n))
	assert.Zero(t, n, "falhas não podem gravar token novo")
}

func TestRepository_Rotate_ReuseRevokesWholeFamily(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid, family := newUserID(t, "ana"), uuid.NewV7()
	require.NoError(t, repo.Create(ctx, newToken(uid, family, "h1", time.Hour)))
	_, err := repo.Rotate(ctx, "h1", next("h2"), time.Now())
	require.NoError(t, err)

	got, err := repo.Rotate(ctx, "h1", next("h3"), time.Now())

	assert.ErrorIs(t, err, auth.ErrTokenReuse)
	assert.Equal(t, uid, got.UserID, "devolve o dono para invalidar os access tokens")
	assert.NotNil(t, revokedAt(t, "h1"))
	assert.NotNil(t, revokedAt(t, "h2"), "o token legítimo da família também é revogado")

	_, err = repo.Rotate(ctx, "h2", next("h4"), time.Now())
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)

	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE token_hash IN ('h3','h4')`).Scan(&n))
	assert.Zero(t, n)
}

func TestRepository_Rotate_ReuseDoesNotTouchOtherFamilies(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "a1", time.Hour)))
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "b1", time.Hour)))
	_, err := repo.Rotate(ctx, "a1", next("a2"), time.Now())
	require.NoError(t, err)

	_, err = repo.Rotate(ctx, "a1", next("a3"), time.Now())
	require.ErrorIs(t, err, auth.ErrTokenReuse)

	assert.Nil(t, revokedAt(t, "b1"), "outra sessão do mesmo usuário continua viva")
}

func TestRepository_Rotate_Concurrent_OnlyOneWins(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "h1", time.Hour)))

	const attempts = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
		reuse   int
		invalid int
	)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Rotate(ctx, "h1", next("n"+string(rune('a'+i))), time.Now())
			mu.Lock()
			defer mu.Unlock()
			switch err {
			case nil:
				success++
			case auth.ErrTokenReuse:
				reuse++
			case auth.ErrInvalidRefreshToken:
				invalid++ // a família já foi revogada pelo perdedor que detectou o reuso
			default:
				t.Errorf("erro inesperado: %v", err)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, success, "só um refresh pode consumir o token")
	assert.Equal(t, 1, reuse, "exatamente um perdedor detecta o reuso e dispara a invalidação")
	assert.Equal(t, attempts-2, invalid)

	var revoked int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL`).Scan(&revoked))
	assert.Equal(t, 2, revoked, "token original e o novo da vencedora ficam revogados")
}

func TestRepository_RevokeFamily(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid, family := newUserID(t, "ana"), uuid.NewV7()
	require.NoError(t, repo.Create(ctx, newToken(uid, family, "a1", time.Hour)))
	_, err := repo.Rotate(ctx, "a1", next("a2"), time.Now())
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "b1", time.Hour)))

	owner, err := repo.RevokeFamily(ctx, "a2", time.Now())

	require.NoError(t, err)
	assert.Equal(t, uid, owner)
	assert.NotNil(t, revokedAt(t, "a1"))
	assert.NotNil(t, revokedAt(t, "a2"))
	assert.Nil(t, revokedAt(t, "b1"))

	_, err = repo.RevokeFamily(ctx, "desconhecido", time.Now())
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
}

func TestRepository_Rotate_PropagaTokenVersion(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	first := newToken(uid, uuid.NewV7(), "a1", time.Hour)
	first.TokenVersion = 3
	require.NoError(t, repo.Create(ctx, first))

	got, err := repo.Rotate(ctx, "a1", next("a2"), time.Now())

	require.NoError(t, err)
	assert.Equal(t, 3, got.TokenVersion)
	var stored int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT token_version FROM refresh_tokens WHERE token_hash = 'a2'`).Scan(&stored))
	assert.Equal(t, 3, stored)
}

func TestRepository_TokensAreDeletedWithUser(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "a1", time.Hour)))

	require.NoError(t, userpg.NewRepository(testPool).Delete(ctx, uid))

	_, err := repo.Rotate(ctx, "a1", next("a2"), time.Now())
	assert.ErrorIs(t, err, auth.ErrInvalidRefreshToken)
}

func TestRepository_Create_DuplicateHashRejected(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	uid := newUserID(t, "ana")
	require.NoError(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "dup", time.Hour)))

	assert.Error(t, repo.Create(ctx, newToken(uid, uuid.NewV7(), "dup", time.Hour)))
}
