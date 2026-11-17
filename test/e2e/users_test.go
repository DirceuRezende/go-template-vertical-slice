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
	"time"

	"users-api/internal/auth"
	authpostgres "users-api/internal/auth/postgres"
	"users-api/internal/httpapi"
	"users-api/internal/plataform/database"
	"users-api/internal/plataform/password"
	"users-api/internal/plataform/token"
	"users-api/internal/testutil"
	"users-api/internal/user"
	"users-api/internal/user/postgres"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pepper = "0123456789abcdef0123456789abcdef"

const (
	adminEmail = "root@example.com"
	adminPass  = "Admin@12345"
	userPass   = "Senha@123"
)

var (
	srv     *httptest.Server
	pool    *pgxpool.Pool
	userSvc *user.Service
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

		tokens, err := token.NewManager(pepper, 15*time.Minute)
		if err != nil {
			panic(err)
		}
		userSvc = user.NewService(postgres.NewRepository(pool), hasher)
		authSvc := auth.NewService(authpostgres.NewRepository(pool), userSvc, hasher, tokens, 7*24*time.Hour)
		srv = httptest.NewServer(httpapi.NewRouter(user.NewHandler(userSvc), auth.NewHandler(authSvc), tokens))
		defer srv.Close()

		return m.Run()
	}()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "TRUNCATE users CASCADE")
	require.NoError(t, err)
}

// call executa a requisição; token (opcional) vai como Bearer.
func call(t *testing.T, method, path, body string, token ...string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if len(token) > 0 && token[0] != "" {
		req.Header.Set("Authorization", "Bearer "+token[0])
	}

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(b)
}

type tokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

func parseTokens(t *testing.T, body string) tokens {
	t.Helper()
	var resp struct{ Data tokens }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	return resp.Data
}

func login(t *testing.T, email, pass string) tokens {
	t.Helper()
	status, body := call(t, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":"`+pass+`"}`)
	require.Equal(t, http.StatusOK, status, body)
	return parseTokens(t, body)
}

func refresh(t *testing.T, refreshToken string) (int, string) {
	t.Helper()
	return call(t, http.MethodPost, "/auth/refresh", `{"refresh_token":"`+refreshToken+`"}`)
}

// createUser cadastra pelo endpoint público e devolve o id.
func createUser(t *testing.T, name string) string {
	t.Helper()
	status, body := call(t, http.MethodPost, "/users",
		`{"name":"`+name+`","email":"`+name+`@example.com","password":"`+userPass+`","age":30}`)
	require.Equal(t, http.StatusCreated, status, body)

	var resp struct{ Data struct{ ID string } }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	return resp.Data.ID
}

// createAdmin garante o admin inicial (como o seed do startup) e faz login.
func createAdmin(t *testing.T) tokens {
	t.Helper()
	require.NoError(t, userSvc.EnsureAdmin(context.Background(), adminEmail, adminPass))
	return login(t, adminEmail, adminPass)
}

func adminID(t *testing.T) string {
	t.Helper()
	u, err := userSvc.FindByEmail(context.Background(), adminEmail)
	require.NoError(t, err)
	return u.ID.String()
}

func TestE2E_Health(t *testing.T) {
	status, body := call(t, http.MethodGet, "/health", "")
	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"status":"ok"}`, body)
}

func TestE2E_CRUDFlow(t *testing.T) {
	reset(t)

	id := createUser(t, "ana")
	tk := login(t, "ana@example.com", userPass).Access

	status, body := call(t, http.MethodGet, "/users/"+id, "", tk)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"name":"ana"`)
	assert.Contains(t, body, `"roles":["user"]`)

	status, body = call(t, http.MethodPut, "/users/"+id, `{"name":"bia","email":"bia@example.com","age":41}`, tk)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"name":"bia"`)

	status, _ = call(t, http.MethodDelete, "/users/"+id, "", tk)
	assert.Equal(t, http.StatusNoContent, status)

	status, _ = call(t, http.MethodGet, "/users/"+id, "", tk)
	assert.Equal(t, http.StatusNotFound, status, "o access token vale até expirar, mas o usuário não existe mais")
}

func TestE2E_List_AdminOnly(t *testing.T) {
	reset(t)
	admin := createAdmin(t)
	createUser(t, "ana")
	createUser(t, "bia")
	ana := login(t, "ana@example.com", userPass)

	status, body := call(t, http.MethodGet, "/users", "", admin.Access)
	assert.Equal(t, http.StatusOK, status)
	var resp struct{ Data []map[string]any }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Len(t, resp.Data, 3)

	status, _ = call(t, http.MethodGet, "/users", "", ana.Access)
	assert.Equal(t, http.StatusForbidden, status)

	status, _ = call(t, http.MethodGet, "/users", "")
	assert.Equal(t, http.StatusUnauthorized, status)
}

func TestE2E_Errors(t *testing.T) {
	reset(t)
	admin := createAdmin(t).Access
	missing := "/users/0190a5c0-0000-7000-8000-000000000000"

	tests := []struct {
		name, method, path, body, token string
		want                            int
	}{
		{"get inexistente", http.MethodGet, missing, "", admin, 404},
		{"get id inválido", http.MethodGet, "/users/abc", "", admin, 400},
		{"put inexistente", http.MethodPut, missing, `{"name":"bia","email":"b@b.com","age":20}`, admin, 404},
		{"delete inexistente", http.MethodDelete, missing, "", admin, 404},
		{"get sem token", http.MethodGet, missing, "", "", 401},
		{"get com token inválido", http.MethodGet, missing, "", "lixo", 401},
		{"create senha fraca", http.MethodPost, "/users", `{"name":"ana","email":"a@a.com","password":"fraca123","age":30}`, "", 422},
		{"create json inválido", http.MethodPost, "/users", `{`, "", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := call(t, tt.method, tt.path, tt.body, tt.token)
			assert.Equal(t, tt.want, status)
		})
	}
}

func TestE2E_PasswordIsHashedAndNeverExposed(t *testing.T) {
	reset(t)
	admin := createAdmin(t).Access
	id := createUser(t, "ana")

	var stored string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT password FROM users WHERE id = $1`, id).Scan(&stored))
	assert.True(t, strings.HasPrefix(stored, "$2"), "esperado hash bcrypt, veio %q", stored)
	assert.NotContains(t, stored, userPass)

	hasher, _ := password.NewHasher(pepper)
	assert.True(t, hasher.Matches(stored, userPass))

	_, one := call(t, http.MethodGet, "/users/"+id, "", admin)
	_, all := call(t, http.MethodGet, "/users", "", admin)
	for _, body := range []string{one, all} {
		assert.NotContains(t, body, "password")
		assert.NotContains(t, body, stored)
	}
}

func TestE2E_RefreshTokenIsStoredOnlyAsHash(t *testing.T) {
	reset(t)
	createUser(t, "ana")

	tk := login(t, "ana@example.com", userPass)

	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, tk.Refresh).Scan(&n))
	assert.Zero(t, n, "o valor em claro não pode estar no banco")
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM refresh_tokens`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestE2E_DuplicateEmailReturnsConflict(t *testing.T) {
	reset(t)
	admin := createAdmin(t).Access
	createUser(t, "ana")

	status, body := call(t, http.MethodPost, "/users",
		`{"name":"outra","email":"ANA@example.com","password":"Senha@123","age":30}`)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body, "Email já cadastrado")

	id := createUser(t, "bia")
	status, _ = call(t, http.MethodPut, "/users/"+id, `{"name":"bia","email":"ana@example.com","age":30}`, admin)
	assert.Equal(t, http.StatusConflict, status)

	_, body = call(t, http.MethodGet, "/users", "", admin)
	var resp struct{ Data []map[string]any }
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Len(t, resp.Data, 3, "admin + ana + bia")
}

func TestE2E_OnlyOwnerOrAdminCanTouchAUser(t *testing.T) {
	reset(t)
	admin := createAdmin(t).Access
	anaID := createUser(t, "ana")
	createUser(t, "bia")
	bia := login(t, "bia@example.com", userPass).Access

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		status, _ := call(t, method, "/users/"+anaID, `{"name":"x","email":"x@x.com","age":20}`, bia)
		assert.Equal(t, http.StatusForbidden, status, method)
	}
	status, _ := call(t, http.MethodGet, "/users/"+anaID, "", admin)
	assert.Equal(t, http.StatusOK, status)
	status, _ = call(t, http.MethodDelete, "/users/"+anaID, "", admin)
	assert.Equal(t, http.StatusNoContent, status)
}

func TestE2E_LoginFailuresAreIndistinguishable(t *testing.T) {
	reset(t)
	createUser(t, "ana")

	s1, b1 := call(t, http.MethodPost, "/auth/login", `{"email":"ana@example.com","password":"Errada@123"}`)
	s2, b2 := call(t, http.MethodPost, "/auth/login", `{"email":"nao@example.com","password":"Senha@123"}`)

	assert.Equal(t, http.StatusUnauthorized, s1)
	assert.Equal(t, http.StatusUnauthorized, s2)
	assert.Equal(t, b1, b2)
}

func TestE2E_RefreshRotationAndReuseDetection(t *testing.T) {
	reset(t)
	createUser(t, "ana")
	first := login(t, "ana@example.com", userPass)

	status, body := refresh(t, first.Refresh)
	require.Equal(t, http.StatusOK, status, body)
	second := parseTokens(t, body)
	assert.NotEqual(t, first.Refresh, second.Refresh)

	status, _ = refresh(t, first.Refresh)
	assert.Equal(t, http.StatusUnauthorized, status, "reuso do token antigo")

	status, _ = refresh(t, second.Refresh)
	assert.Equal(t, http.StatusUnauthorized, status, "a sessão inteira foi revogada")

	again := login(t, "ana@example.com", userPass)
	status, _ = refresh(t, again.Refresh)
	assert.Equal(t, http.StatusOK, status, "novo login volta a funcionar")
}

func TestE2E_ExpiredRefreshTokenIsRejected(t *testing.T) {
	reset(t)
	createUser(t, "ana")
	tk := login(t, "ana@example.com", userPass)
	_, err := pool.Exec(context.Background(), `UPDATE refresh_tokens SET expires_at = now() - interval '1 second'`)
	require.NoError(t, err)

	status, _ := refresh(t, tk.Refresh)

	assert.Equal(t, http.StatusUnauthorized, status)
}

func TestE2E_Logout(t *testing.T) {
	reset(t)
	id := createUser(t, "ana")
	tk := login(t, "ana@example.com", userPass)
	status, _ := call(t, http.MethodGet, "/users/"+id, "", tk.Access)
	require.Equal(t, http.StatusOK, status)

	status, _ = call(t, http.MethodPost, "/auth/logout", `{"refresh_token":"`+tk.Refresh+`"}`)
	assert.Equal(t, http.StatusNoContent, status)

	status, _ = call(t, http.MethodGet, "/users/"+id, "", tk.Access)
	assert.Equal(t, http.StatusOK, status, "stateless: o access token vale até expirar")
	status, _ = refresh(t, tk.Refresh)
	assert.Equal(t, http.StatusUnauthorized, status, "refresh token revogado")
}

func TestE2E_ChangePassword(t *testing.T) {
	reset(t)
	id := createUser(t, "ana")
	tk := login(t, "ana@example.com", userPass)
	other := login(t, "ana@example.com", userPass) // segunda sessão
	change := `{"current_password":"` + userPass + `","new_password":"Nova@45678"}`

	status, _ := call(t, http.MethodPost, "/auth/change-password",
		`{"current_password":"Errada@123","new_password":"Nova@45678"}`, tk.Access)
	assert.Equal(t, http.StatusBadRequest, status)

	status, _ = call(t, http.MethodPost, "/auth/change-password",
		`{"current_password":"`+userPass+`","new_password":"fraca"}`, tk.Access)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	status, _ = call(t, http.MethodPost, "/auth/change-password", change, tk.Access)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = refresh(t, tk.Refresh)
	assert.Equal(t, http.StatusUnauthorized, status, "refresh da sessão que trocou a senha")
	status, _ = refresh(t, other.Refresh)
	assert.Equal(t, http.StatusUnauthorized, status, "refresh das outras sessões (token_version mudou)")

	status, _ = call(t, http.MethodPost, "/auth/login", `{"email":"ana@example.com","password":"`+userPass+`"}`)
	assert.Equal(t, http.StatusUnauthorized, status, "senha antiga")
	fresh := login(t, "ana@example.com", "Nova@45678")
	status, _ = call(t, http.MethodGet, "/users/"+id, "", fresh.Access)
	assert.Equal(t, http.StatusOK, status)
}

func TestE2E_AdminManagesRoles(t *testing.T) {
	reset(t)
	admin := createAdmin(t)
	anaID := createUser(t, "ana")
	ana := login(t, "ana@example.com", userPass)
	rolesURL := "/users/" + anaID + "/roles"

	status, _ := call(t, http.MethodPut, rolesURL, `{"roles":["user","admin"]}`, ana.Access)
	assert.Equal(t, http.StatusForbidden, status, "usuário comum não se promove")

	status, body := call(t, http.MethodPut, rolesURL, `{"roles":["user","admin","editor"]}`, admin.Access)
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, `"roles":["admin","editor","user"]`)

	status, _ = call(t, http.MethodGet, "/users", "", ana.Access)
	assert.Equal(t, http.StatusForbidden, status, "o access token antigo ainda leva os roles antigos")

	status, body = refresh(t, ana.Refresh)
	require.Equal(t, http.StatusOK, status, body)
	promoted := parseTokens(t, body)
	status, _ = call(t, http.MethodGet, "/users", "", promoted.Access)
	assert.Equal(t, http.StatusOK, status, "o refresh traz os roles novos")

	status, _ = call(t, http.MethodPut, rolesURL, `{"roles":["Inválido"]}`, promoted.Access)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// rebaixada pelo admin original: o próximo refresh já vem sem admin.
	status, _ = call(t, http.MethodPut, rolesURL, `{"roles":["user"]}`, admin.Access)
	require.Equal(t, http.StatusOK, status)
	status, body = refresh(t, promoted.Refresh)
	require.Equal(t, http.StatusOK, status, body)
	status, _ = call(t, http.MethodGet, "/users", "", parseTokens(t, body).Access)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestE2E_LastAdminIsProtected(t *testing.T) {
	reset(t)
	admin := createAdmin(t).Access
	root := adminID(t)

	status, body := call(t, http.MethodPut, "/users/"+root+"/roles", `{"roles":["user"]}`, admin)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body, "último administrador")

	status, body = call(t, http.MethodDelete, "/users/"+root, "", admin)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body, "último administrador")

	status, _ = call(t, http.MethodGet, "/users/"+root, "", admin)
	assert.Equal(t, http.StatusOK, status, "nada foi alterado")
}

func TestE2E_SignupCannotGrantRoles(t *testing.T) {
	reset(t)

	status, body := call(t, http.MethodPost, "/users",
		`{"name":"ana","email":"ana@example.com","password":"Senha@123","age":30,"roles":["admin"]}`)

	require.Equal(t, http.StatusCreated, status)
	assert.Contains(t, body, `"roles":["user"]`)
	tk := login(t, "ana@example.com", userPass)
	status, _ = call(t, http.MethodGet, "/users", "", tk.Access)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestE2E_EnsureAdminIsIdempotent(t *testing.T) {
	reset(t)
	createAdmin(t)

	require.NoError(t, userSvc.EnsureAdmin(context.Background(), adminEmail, "Outra@12345"))

	login(t, adminEmail, adminPass) // senha original continua valendo
	var admins int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM user_roles WHERE role = 'admin'`).Scan(&admins))
	assert.Equal(t, 1, admins)
}
