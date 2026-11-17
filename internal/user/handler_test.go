package user_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"users-api/internal/plataform/authn"
	"users-api/internal/plataform/token"
	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type env struct {
	router http.Handler
	repo   *usertest.FakeRepository
	svc    *user.Service
	tokens *token.Manager
}

func newEnv(t *testing.T) env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, user.RegisterValidations())

	repo := usertest.NewFakeRepository()
	svc := user.NewService(repo, newHasher(t))
	tokens, err := token.NewManager(testPepper, 15*time.Minute)
	require.NoError(t, err)

	r := gin.New()
	user.NewHandler(svc).RegisterRoutes(r, authn.Middleware(tokens))
	return env{router: r, repo: repo, svc: svc, tokens: tokens}
}

// bearer emite um access token com os roles que o usuário tem agora, como o
// login faria. O token não muda se os roles mudarem depois.
func (e env) bearer(t *testing.T, id uuid.UUID) string {
	t.Helper()
	u, err := e.svc.Get(t.Context(), id)
	require.NoError(t, err)
	raw, err := e.tokens.Issue(id, u.Roles)
	require.NoError(t, err)
	return raw
}

// as devolve o header Authorization de um usuário.
func (e env) as(t *testing.T, id uuid.UUID) string { return "Bearer " + e.bearer(t, id) }

// admin devolve o admin do ambiente, criando-o na primeira chamada.
func (e env) admin(t *testing.T) uuid.UUID {
	t.Helper()
	if existing, err := e.svc.FindByEmail(t.Context(), "root@example.com"); err == nil {
		return existing.ID
	}
	u, err := e.svc.Create(t.Context(), user.Input{Name: "Root", Email: "root@example.com", Password: "Senha@123", Age: 40})
	require.NoError(t, err)
	_, err = e.svc.SetRoles(t.Context(), u.ID, []string{user.RoleUser, user.RoleAdmin})
	require.NoError(t, err)
	return u.ID
}

func (e env) user(t *testing.T, email string) uuid.UUID {
	t.Helper()
	return createWithEmail(t, e.router, email)
}

// do executa a requisição; auth (opcional) é o valor do header Authorization.
func do(r http.Handler, method, path, body string, auth ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if len(auth) > 0 {
		req.Header.Set("Authorization", auth[0])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const validBody = `{"name":"Ana","email":"ana@example.com","password":"Senha@123","age":30}`

func createWithEmail(t *testing.T, r http.Handler, email string) uuid.UUID {
	t.Helper()
	w := do(r, http.MethodPost, "/users",
		`{"name":"Ana","email":"`+email+`","password":"Senha@123","age":30}`)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct{ Data user.UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Data.ID
}

func TestHandler_Create(t *testing.T) {
	e := newEnv(t)

	w := do(e.router, http.MethodPost, "/users", validBody)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.NotContains(t, w.Body.String(), "password")
	assert.NotContains(t, w.Body.String(), "Senha@123")
	assert.Contains(t, w.Body.String(), `"name":"Ana"`)
	assert.Contains(t, w.Body.String(), `"roles":["user"]`)
}

func TestHandler_Create_IgnoresRolesInBody(t *testing.T) {
	e := newEnv(t)

	w := do(e.router, http.MethodPost, "/users",
		`{"name":"Ana","email":"ana@example.com","password":"Senha@123","age":30,"roles":["admin"]}`)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"roles":["user"]`)
}

func TestHandler_Create_Invalid(t *testing.T) {
	e := newEnv(t)

	tests := []struct {
		name string
		body string
		code int
	}{
		{"senha fraca", `{"name":"Ana","email":"a@a.com","password":"fraca123","age":30}`, 422},
		{"email inválido", `{"name":"Ana","email":"x","password":"Senha@123","age":30}`, 422},
		{"nome curto", `{"name":"A","email":"a@a.com","password":"Senha@123","age":30}`, 422},
		{"idade zero", `{"name":"Ana","email":"a@a.com","password":"Senha@123","age":0}`, 422},
		{"campos faltando", `{}`, 422},
		{"json malformado", `{"name":`, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := do(e.router, http.MethodPost, "/users", tt.body)
			assert.Equal(t, tt.code, w.Code)
		})
	}
}

func TestHandler_ProtectedRoutes_RequireAuthentication(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com").String()

	tests := []struct{ method, path string }{
		{http.MethodGet, "/users"},
		{http.MethodGet, "/users/" + id},
		{http.MethodPut, "/users/" + id},
		{http.MethodDelete, "/users/" + id},
		{http.MethodPut, "/users/" + id + "/roles"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			assert.Equal(t, http.StatusUnauthorized, do(e.router, tt.method, tt.path, `{}`).Code)
			assert.Equal(t, http.StatusUnauthorized, do(e.router, tt.method, tt.path, `{}`, "Bearer lixo").Code)
		})
	}
}

func TestHandler_Get(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com")
	auth := e.as(t, id)

	t.Run("ok", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users/"+id.String(), "", auth)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), id.String())
		assert.NotContains(t, w.Body.String(), "password")
	})
	t.Run("id inválido", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users/abc", "", auth)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("outro usuário não é permitido", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users/"+uuid.New().String(), "", auth)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("admin acessa qualquer um", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users/"+id.String(), "", e.as(t, e.admin(t)))
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("admin com id inexistente", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users/"+uuid.New().String(), "", e.as(t, e.admin(t)))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_GetAll(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	regular := e.user(t, "b@example.com")

	t.Run("usuário comum recebe 403", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, do(e.router, http.MethodGet, "/users", "", e.as(t, regular)).Code)
	})
	t.Run("admin lista todos", func(t *testing.T) {
		w := do(e.router, http.MethodGet, "/users", "", e.as(t, admin))
		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct{ Data []user.UserResponse }
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp.Data, 2)
		assert.NotContains(t, w.Body.String(), "password")
	})
}

func TestHandler_Update(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com")
	auth := e.as(t, id)
	body := `{"name":"Bia","email":"bia@example.com","age":41}`

	t.Run("ok", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), body, auth)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"name":"Bia"`)
	})
	t.Run("outro usuário não é permitido", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+uuid.New().String(), body, auth)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("admin não encontrado", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+uuid.New().String(), body, e.as(t, e.admin(t)))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("admin altera outro usuário", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), body, e.as(t, e.admin(t)))
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("id inválido", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/abc", body, auth)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("validação", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), `{"name":"B","email":"x","age":0}`, auth)
		assert.Equal(t, 422, w.Code)
	})
	t.Run("json malformado", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), `{`, auth)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Delete(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com")
	other := e.user(t, "outro@example.com")
	auth := e.as(t, id)

	assert.Equal(t, http.StatusForbidden, do(e.router, http.MethodDelete, "/users/"+other.String(), "", auth).Code)
	assert.Equal(t, http.StatusNoContent, do(e.router, http.MethodDelete, "/users/"+id.String(), "", auth).Code)
	assert.Equal(t, http.StatusBadRequest, do(e.router, http.MethodDelete, "/users/abc", "", e.as(t, other)).Code)

	t.Run("token de usuário excluído vale até expirar, mas não acha o recurso", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, do(e.router, http.MethodGet, "/users/"+id.String(), "", auth).Code)
	})
}

func TestHandler_Delete_LastAdmin(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)

	w := do(e.router, http.MethodDelete, "/users/"+admin.String(), "", e.as(t, admin))

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), user.ErrLastAdmin.Error())
}

func TestHandler_UpdateRoles(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	target := e.user(t, "ana@example.com")
	url := "/users/" + target.String() + "/roles"

	t.Run("usuário comum recebe 403, mesmo no próprio id", func(t *testing.T) {
		w := do(e.router, http.MethodPut, url, `{"roles":["user","admin"]}`, e.as(t, target))
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("admin define roles, criando os novos", func(t *testing.T) {
		w := do(e.router, http.MethodPut, url, `{"roles":["user","editor","editor"]}`, e.as(t, admin))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"roles":["editor","user"]`)
	})
	t.Run("roles novos só valem num access token emitido depois", func(t *testing.T) {
		stale := e.as(t, target)
		require.Equal(t, http.StatusOK, do(e.router, http.MethodPut, url, `{"roles":["user","admin"]}`, e.as(t, admin)).Code)

		assert.Equal(t, http.StatusForbidden, do(e.router, http.MethodGet, "/users", "", stale).Code)
		assert.Equal(t, http.StatusOK, do(e.router, http.MethodGet, "/users", "", e.as(t, target)).Code)

		require.Equal(t, http.StatusOK, do(e.router, http.MethodPut, url, `{"roles":["user"]}`, e.as(t, admin)).Code)
	})
	t.Run("usuário inexistente", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+uuid.New().String()+"/roles", `{"roles":["user"]}`, e.as(t, admin))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("id inválido", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, do(e.router, http.MethodPut, "/users/abc/roles", `{"roles":["user"]}`, e.as(t, admin)).Code)
	})
	t.Run("não remove o último admin", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+admin.String()+"/roles", `{"roles":["user"]}`, e.as(t, admin))
		assert.Equal(t, http.StatusConflict, w.Code)
	})
	t.Run("validação", func(t *testing.T) {
		for name, body := range map[string]string{
			"vazio":           `{"roles":[]}`,
			"ausente":         `{}`,
			"nome inválido":   `{"roles":["Admin!"]}`,
			"nome curto":      `{"roles":["a"]}`,
			"json malformado": `{`,
		} {
			w := do(e.router, http.MethodPut, url, body, e.as(t, admin))
			assert.Contains(t, []int{http.StatusUnprocessableEntity, http.StatusBadRequest}, w.Code, name)
			assert.NotEqual(t, http.StatusOK, w.Code, name)
		}
	})
}

func TestHandler_UpdateRoles_ValidationMessage(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)

	w := do(e.router, http.MethodPut, "/users/"+admin.String()+"/roles", `{"roles":["user","Inválido"]}`, e.as(t, admin))

	require.Equal(t, 422, w.Code)
	assert.JSONEq(t, `{"error":"dados inválidos","fields":[{"field":"roles[1]","message":"deve ter de 2 a 32 caracteres: letras minúsculas, números e _"}]}`, w.Body.String())
}

func TestHandler_Create_EmailTaken(t *testing.T) {
	e := newEnv(t)
	e.user(t, "ana@example.com")

	for _, email := range []string{"ana@example.com", "ANA@Example.com"} {
		body := `{"name":"Outra","email":"` + email + `","password":"Senha@123","age":25}`
		w := do(e.router, http.MethodPost, "/users", body)

		assert.Equal(t, http.StatusConflict, w.Code, email)
		assert.Contains(t, w.Body.String(), user.ErrEmailTaken.Error())
	}
}

func TestHandler_Update_Email(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com")
	e.user(t, "bia@example.com")
	auth := e.as(t, id)

	t.Run("email de outro usuário", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), `{"name":"Ana","email":"bia@example.com","age":30}`, auth)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
	t.Run("mantendo o próprio email", func(t *testing.T) {
		w := do(e.router, http.MethodPut, "/users/"+id.String(), `{"name":"Ana Maria","email":"ana@example.com","age":30}`, auth)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandler_ValidationErrorsAreDetailed(t *testing.T) {
	e := newEnv(t)

	w := do(e.router, http.MethodPost, "/users", `{"name":"A","email":"x","password":"fraca123","age":0}`)

	require.Equal(t, 422, w.Code)
	assert.JSONEq(t, `{
		"error": "dados inválidos",
		"fields": [
			{"field": "name", "message": "deve ter no mínimo 2 caracteres"},
			{"field": "password", "message": "deve conter ao menos uma letra maiúscula, uma minúscula e um símbolo (@$#%&)"},
			{"field": "email", "message": "deve ser um email válido"},
			{"field": "age", "message": "campo obrigatório"}
		]
	}`, w.Body.String())
	assert.NotContains(t, w.Body.String(), "Key:")
}

func TestHandler_ValidationErrors_Update(t *testing.T) {
	e := newEnv(t)
	id := e.user(t, "ana@example.com")

	w := do(e.router, http.MethodPut, "/users/"+id.String(), `{"name":"Ana","email":"ana@example.com","age":200}`, e.as(t, id))

	require.Equal(t, 422, w.Code)
	assert.JSONEq(t, `{"error":"dados inválidos","fields":[{"field":"age","message":"deve ser no máximo 150"}]}`, w.Body.String())
}

func TestHandler_MalformedJSON_Message(t *testing.T) {
	e := newEnv(t)

	w := do(e.router, http.MethodPost, "/users", `{"name":`)

	require.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "corpo da requisição inválido")
}
