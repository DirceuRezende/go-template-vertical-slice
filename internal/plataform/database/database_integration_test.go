//go:build integration

package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"users-api/internal/plataform/database"
	"users-api/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dbURL string

func TestMain(m *testing.M) {
	url, terminate, err := testutil.StartPostgres(context.Background())
	if err != nil {
		panic(err)
	}
	dbURL = url
	code := m.Run()
	terminate()
	os.Exit(code)
}

func TestMigrateUp_IsIdempotent(t *testing.T) {
	require.NoError(t, database.MigrateUp(dbURL))
	require.NoError(t, database.MigrateUp(dbURL))

	pool, err := database.NewPool(context.Background(), database.DefaultConfig(dbURL, "it"))
	require.NoError(t, err)
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')`).Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMigrateUp_CreatesEmailUniqueIndex(t *testing.T) {
	require.NoError(t, database.MigrateUp(dbURL))

	pool, err := database.NewPool(context.Background(), database.DefaultConfig(dbURL, "it"))
	require.NoError(t, err)
	defer pool.Close()

	var def string
	err = pool.QueryRow(context.Background(),
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'users_email_unique_idx'`).Scan(&def)
	require.NoError(t, err)
	assert.Contains(t, def, "UNIQUE")
	assert.Contains(t, def, "lower(email)")
}

func TestMigrateUp_InvalidURL(t *testing.T) {
	assert.Error(t, database.MigrateUp("postgres://x:y@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"))
}

func TestNewPool_SetsAppName(t *testing.T) {
	pool, err := database.NewPool(context.Background(), database.DefaultConfig(dbURL, "meu-app"))
	require.NoError(t, err)
	defer pool.Close()

	var name string
	require.NoError(t, pool.QueryRow(context.Background(), `SHOW application_name`).Scan(&name))
	assert.Equal(t, "meu-app", name)
}

func TestNewPool_Errors(t *testing.T) {
	t.Run("url inválida", func(t *testing.T) {
		_, err := database.NewPool(context.Background(), database.DefaultConfig("://bad", "x"))
		assert.Error(t, err)
	})
	t.Run("sem conexão", func(t *testing.T) {
		cfg := database.DefaultConfig("postgres://x:y@127.0.0.1:1/none?sslmode=disable", "x")
		cfg.ConnectTimeout = time.Second
		_, err := database.NewPool(context.Background(), cfg)
		assert.Error(t, err)
	})
}
