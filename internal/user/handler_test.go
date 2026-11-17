package user_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouter(t *testing.T) (*gin.Engine, *usertest.FakeRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, user.RegisterValidations())

	repo := usertest.NewFakeRepository()
	h := user.NewHandler(user.NewService(repo, newHasher(t)))
	r := gin.New()
	h.RegisterRoutes(r)
	return r, repo
}

func do(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const validBody = `{"name":"Ana","email":"ana@example.com","password":"Senha@123","age":30}`

func create(t *testing.T, r http.Handler) uuid.UUID {
	t.Helper()
	return createWithEmail(t, r, "ana@example.com")
}

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
	r, _ := newRouter(t)

	w := do(r, http.MethodPost, "/users", validBody)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.NotContains(t, w.Body.String(), "password")
	assert.NotContains(t, w.Body.String(), "Senha@123")
	assert.Contains(t, w.Body.String(), `"name":"Ana"`)
}

func TestHandler_Create_Invalid(t *testing.T) {
	r, _ := newRouter(t)

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
			w := do(r, http.MethodPost, "/users", tt.body)
			assert.Equal(t, tt.code, w.Code)
		})
	}
}

func TestHandler_Get(t *testing.T) {
	r, _ := newRouter(t)
	id := create(t, r)

	t.Run("ok", func(t *testing.T) {
		w := do(r, http.MethodGet, "/users/"+id.String(), "")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), id.String())
	})
	t.Run("não encontrado", func(t *testing.T) {
		w := do(r, http.MethodGet, "/users/"+uuid.New().String(), "")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("id inválido", func(t *testing.T) {
		w := do(r, http.MethodGet, "/users/abc", "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_GetAll(t *testing.T) {
	r, _ := newRouter(t)

	w := do(r, http.MethodGet, "/users", "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"data":[]}`, w.Body.String())

	createWithEmail(t, r, "a@example.com")
	createWithEmail(t, r, "b@example.com")

	w = do(r, http.MethodGet, "/users", "")
	var resp struct{ Data []user.UserResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 2)
}

func TestHandler_Update(t *testing.T) {
	r, _ := newRouter(t)
	id := create(t, r)
	body := `{"name":"Bia","email":"bia@example.com","age":41}`

	t.Run("ok", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+id.String(), body)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"name":"Bia"`)
	})
	t.Run("não encontrado", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+uuid.New().String(), body)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("id inválido", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/abc", body)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("validação", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+id.String(), `{"name":"B","email":"x","age":0}`)
		assert.Equal(t, 422, w.Code)
	})
	t.Run("json malformado", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+id.String(), `{`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Delete(t *testing.T) {
	r, _ := newRouter(t)
	id := create(t, r)

	assert.Equal(t, http.StatusNoContent, do(r, http.MethodDelete, "/users/"+id.String(), "").Code)
	assert.Equal(t, http.StatusNotFound, do(r, http.MethodDelete, "/users/"+id.String(), "").Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodDelete, "/users/abc", "").Code)
}

func TestHandler_Create_EmailTaken(t *testing.T) {
	r, _ := newRouter(t)
	create(t, r)

	for _, email := range []string{"ana@example.com", "ANA@Example.com"} {
		body := `{"name":"Outra","email":"` + email + `","password":"Senha@123","age":25}`
		w := do(r, http.MethodPost, "/users", body)

		assert.Equal(t, http.StatusConflict, w.Code, email)
		assert.Contains(t, w.Body.String(), user.ErrEmailTaken.Error())
	}
}

func TestHandler_Update_Email(t *testing.T) {
	r, _ := newRouter(t)
	id := create(t, r)
	w := do(r, http.MethodPost, "/users", `{"name":"Bia","email":"bia@example.com","password":"Senha@123","age":25}`)
	require.Equal(t, http.StatusCreated, w.Code)

	t.Run("email de outro usuário", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+id.String(), `{"name":"Ana","email":"bia@example.com","age":30}`)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
	t.Run("mantendo o próprio email", func(t *testing.T) {
		w := do(r, http.MethodPut, "/users/"+id.String(), `{"name":"Ana Maria","email":"ana@example.com","age":30}`)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandler_ValidationErrorsAreDetailed(t *testing.T) {
	r, _ := newRouter(t)

	w := do(r, http.MethodPost, "/users", `{"name":"A","email":"x","password":"fraca123","age":0}`)

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
	r, _ := newRouter(t)
	id := create(t, r)

	w := do(r, http.MethodPut, "/users/"+id.String(), `{"name":"Ana","email":"ana@example.com","age":200}`)

	require.Equal(t, 422, w.Code)
	assert.JSONEq(t, `{"error":"dados inválidos","fields":[{"field":"age","message":"deve ser no máximo 150"}]}`, w.Body.String())
}

func TestHandler_MalformedJSON_Message(t *testing.T) {
	r, _ := newRouter(t)

	w := do(r, http.MethodPost, "/users", `{"name":`)

	require.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "corpo da requisição inválido")
}
