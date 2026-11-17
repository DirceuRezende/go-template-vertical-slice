package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"users-api/internal/auth"
	"users-api/internal/plataform/authn"
	"users-api/internal/user"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRouter(t *testing.T) (http.Handler, fixture) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, user.RegisterValidations())

	f := newFixture(t, time.Hour)
	r := gin.New()
	auth.NewHandler(f.svc).RegisterRoutes(r, authn.Middleware(f.tokens))
	return r, f
}

func do(r http.Handler, method, path, body string, bearer ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if len(bearer) > 0 {
		req.Header.Set("Authorization", "Bearer "+bearer[0])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func login(t *testing.T, r http.Handler) auth.TokenResponse {
	t.Helper()
	w := do(r, http.MethodPost, "/auth/login", `{"email":"ana@example.com","password":"`+goodPass+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct{ Data auth.TokenResponse }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Data
}

func TestHandler_Login(t *testing.T) {
	r, _ := newRouter(t)

	w := do(r, http.MethodPost, "/auth/login", `{"email":"ana@example.com","password":"`+goodPass+`"}`)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct{ Data map[string]any }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Bearer", resp.Data["token_type"])
	assert.EqualValues(t, 900, resp.Data["expires_in"])
	assert.NotEmpty(t, resp.Data["access_token"])
	assert.NotEmpty(t, resp.Data["refresh_token"])
	assert.NotContains(t, w.Body.String(), "password")
}

func TestHandler_Login_Failures(t *testing.T) {
	r, _ := newRouter(t)

	tests := []struct {
		name, body string
		code       int
	}{
		{"senha errada", `{"email":"ana@example.com","password":"Errada@123"}`, 401},
		{"email inexistente", `{"email":"x@example.com","password":"Senha@123"}`, 401},
		{"email inválido", `{"email":"x","password":"Senha@123"}`, 422},
		{"campos faltando", `{}`, 422},
		{"json malformado", `{`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, do(r, http.MethodPost, "/auth/login", tt.body).Code)
		})
	}

	t.Run("mesma resposta para senha errada e e-mail inexistente", func(t *testing.T) {
		a := do(r, http.MethodPost, "/auth/login", tests[0].body)
		b := do(r, http.MethodPost, "/auth/login", tests[1].body)
		assert.Equal(t, a.Body.String(), b.Body.String())
	})
}

func TestHandler_Refresh(t *testing.T) {
	r, _ := newRouter(t)
	tokens := login(t, r)

	w := do(r, http.MethodPost, "/auth/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), tokens.RefreshToken)

	t.Run("reuso é rejeitado", func(t *testing.T) {
		w := do(r, http.MethodPost, "/auth/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), auth.ErrInvalidRefreshToken.Error())
	})
	t.Run("validação", func(t *testing.T) {
		assert.Equal(t, 422, do(r, http.MethodPost, "/auth/refresh", `{}`).Code)
		assert.Equal(t, 400, do(r, http.MethodPost, "/auth/refresh", `{`).Code)
	})
}

func TestHandler_Logout(t *testing.T) {
	r, _ := newRouter(t)
	tokens := login(t, r)
	body := `{"refresh_token":"` + tokens.RefreshToken + `"}`

	assert.Equal(t, http.StatusNoContent, do(r, http.MethodPost, "/auth/logout", body).Code)
	assert.Equal(t, http.StatusNoContent, do(r, http.MethodPost, "/auth/logout", body).Code, "idempotente")
	assert.Equal(t, http.StatusUnauthorized, do(r, http.MethodPost, "/auth/refresh", body).Code)
	assert.Equal(t, 422, do(r, http.MethodPost, "/auth/logout", `{}`).Code)
}

func TestHandler_ChangePassword(t *testing.T) {
	r, _ := newRouter(t)
	tokens := login(t, r)
	body := `{"current_password":"Senha@123","new_password":"Nova@4567"}`

	t.Run("exige autenticação", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, do(r, http.MethodPost, "/auth/change-password", body).Code)
	})
	t.Run("senha atual errada", func(t *testing.T) {
		w := do(r, http.MethodPost, "/auth/change-password",
			`{"current_password":"Errada@123","new_password":"Nova@4567"}`, tokens.AccessToken)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), auth.ErrWrongPassword.Error())
	})
	t.Run("nova senha fraca", func(t *testing.T) {
		w := do(r, http.MethodPost, "/auth/change-password",
			`{"current_password":"Senha@123","new_password":"fraca123"}`, tokens.AccessToken)
		require.Equal(t, 422, w.Code)
		assert.Contains(t, w.Body.String(), `"field":"new_password"`)
	})
	t.Run("sucesso invalida o refresh antigo (o access vale até expirar)", func(t *testing.T) {
		w := do(r, http.MethodPost, "/auth/change-password", body, tokens.AccessToken)
		require.Equal(t, http.StatusNoContent, w.Code)

		assert.Equal(t, http.StatusUnauthorized,
			do(r, http.MethodPost, "/auth/refresh", `{"refresh_token":"`+tokens.RefreshToken+`"}`).Code)

		ok := do(r, http.MethodPost, "/auth/login", `{"email":"ana@example.com","password":"Nova@4567"}`)
		assert.Equal(t, http.StatusOK, ok.Code)
	})
}
