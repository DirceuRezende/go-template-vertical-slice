package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"users-api/internal/auth"
	"users-api/internal/auth/authtest"
	"users-api/internal/httpapi"
	"users-api/internal/plataform/password"
	"users-api/internal/plataform/token"
	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "0123456789abcdef0123456789abcdef"

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	hasher, err := password.NewHasher(secret)
	require.NoError(t, err)
	tokens, err := token.NewManager(secret, time.Minute)
	require.NoError(t, err)

	svc := user.NewService(usertest.NewFakeRepository(), hasher)
	authSvc := auth.NewService(authtest.NewFakeRepository(), svc, hasher, tokens, time.Hour)

	return httpapi.NewRouter(user.NewHandler(svc), auth.NewHandler(authSvc), tokens)
}

func TestRouter_Health(t *testing.T) {
	w := httptest.NewRecorder()
	newRouter(t).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestRouter_ProtectedRoutesRequireToken(t *testing.T) {
	r := newRouter(t)

	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/users"},
		{http.MethodGet, "/users/0190a5c0-0000-7000-8000-000000000000"},
		{http.MethodPut, "/users/0190a5c0-0000-7000-8000-000000000000"},
		{http.MethodDelete, "/users/0190a5c0-0000-7000-8000-000000000000"},
		{http.MethodPut, "/users/0190a5c0-0000-7000-8000-000000000000/roles"},
		{http.MethodPost, "/auth/change-password"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestRouter_PublicRoutesDoNotRequireToken(t *testing.T) {
	r := newRouter(t)

	for _, path := range []string{"/users", "/auth/login", "/auth/refresh", "/auth/logout"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
			// Sem corpo falha na validação (400/422), mas nunca com 401 de middleware.
			assert.NotEqual(t, http.StatusUnauthorized, w.Code)
			assert.NotEqual(t, http.StatusNotFound, w.Code)
		})
	}
}
