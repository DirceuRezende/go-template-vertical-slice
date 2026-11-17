//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"users-api/internal/httpapi"
	"users-api/internal/plataform/database"
	"users-api/internal/plataform/password"
	"users-api/internal/testutil"
	"users-api/internal/user"
	"users-api/internal/user/postgres"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pepper = "0123456789abcdef0123456789abcdef"

var (
	srv  *httptest.Server
	pool *pgxpool.Pool
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	url, terminate, err := testutil.StartPostgres(ctx)
	if err != nil {
		panic(err)
	}

	code := func() int {
		defer terminate()
		gin.SetMode(gin.TestMode)

		if err := database.MigrateUp(url); err != nil {
			panic(err)
		}
		if pool, err = database.NewPool(ctx, database.DefaultConfig(url, "e2e")); err != nil {
			panic(err)
		}
		defer pool.Close()
		if err := user.RegisterValidations(); err != nil {
			panic(err)
		}
		hasher, err := password.NewHasher(pepper)
		if err != nil {
			panic(err)
		}

		handler := user.NewHandler(user.NewService(postgres.NewRepository(pool), hasher))
		srv = httptest.NewServer(httpapi.NewRouter(handler))
		defer srv.Close()

		return m.Run()
	}()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE users")
	require.NoError(t, err)
}

func call(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(b)
}

func createUser(t *testing.T, name string) string {
	t.Helper()
	status, body := call(t, http.MethodPost, "/users",
		`{"name":"`+name+`","email":"`+name+`@example.com","password":"Senha@123","age":30}`)
	require.Equal(t, http.StatusCreated, status, body)

	var resp struct{ Data struct{ ID string } }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	return resp.Data.ID
}

func TestE2E_Health(t *testing.T) {
	status, body := call(t, http.MethodGet, "/health", "")
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"status":"ok"}`, body)
}

func TestE2E_CRUDFlow(t *testing.T) {
	reset(t)

	id := createUser(t, "ana")

	status, body := call(t, http.MethodGet, "/users/"+id, "")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"name":"ana"`)

	status, body = call(t, http.MethodPut, "/users/"+id, `{"name":"bia","email":"bia@example.com","age":41}`)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"name":"bia"`)

	status, body = call(t, http.MethodGet, "/users", "")
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, id)

	status, _ = call(t, http.MethodDelete, "/users/"+id, "")
	assert.Equal(t, http.StatusNoContent, status)

	status, _ = call(t, http.MethodGet, "/users/"+id, "")
	assert.Equal(t, http.StatusNotFound, status)
}

func TestE2E_List(t *testing.T) {
	reset(t)

	status, body := call(t, http.MethodGet, "/users", "")
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"data":[]}`, body)

	createUser(t, "ana")
	createUser(t, "bia")
	createUser(t, "caio")

	_, body = call(t, http.MethodGet, "/users", "")
	var resp struct{ Data []map[string]any }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Len(t, resp.Data, 3)
}

func TestE2E_Errors(t *testing.T) {
	reset(t)
	missing := "/users/0190a5c0-0000-7000-8000-000000000000"

	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"get inexistente", http.MethodGet, missing, "", 404},
		{"get id inválido", http.MethodGet, "/users/abc", "", 400},
		{"put inexistente", http.MethodPut, missing, `{"name":"bia","email":"b@b.com","age":20}`, 404},
		{"delete inexistente", http.MethodDelete, missing, "", 404},
		{"create senha fraca", http.MethodPost, "/users", `{"name":"ana","email":"a@a.com","password":"fraca123","age":30}`, 422},
		{"create json inválido", http.MethodPost, "/users", `{`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := call(t, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.want, status)
		})
	}
}

func TestE2E_PasswordIsHashedAndNeverExposed(t *testing.T) {
	reset(t)
	id := createUser(t, "ana")

	var stored string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT password FROM users WHERE id = $1`, id).Scan(&stored))
	assert.True(t, strings.HasPrefix(stored, "$2"), "esperado hash bcrypt, veio %q", stored)
	assert.NotContains(t, stored, "Senha@123")

	hasher, _ := password.NewHasher(pepper)
	assert.True(t, hasher.Matches(stored, "Senha@123"))

	_, one := call(t, http.MethodGet, "/users/"+id, "")
	_, all := call(t, http.MethodGet, "/users", "")
	for _, body := range []string{one, all} {
		assert.NotContains(t, body, "password")
		assert.NotContains(t, body, stored)
	}
}

func TestE2E_DuplicateEmailReturnsConflict(t *testing.T) {
	reset(t)
	createUser(t, "ana")

	status, body := call(t, http.MethodPost, "/users",
		`{"name":"outra","email":"ANA@example.com","password":"Senha@123","age":30}`)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body, "Email já cadastrado")

	id := createUser(t, "bia")
	status, _ = call(t, http.MethodPut, "/users/"+id, `{"name":"bia","email":"ana@example.com","age":30}`)
	assert.Equal(t, http.StatusConflict, status)

	_, body = call(t, http.MethodGet, "/users", "")
	var resp struct{ Data []map[string]any }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Len(t, resp.Data, 2)
}
