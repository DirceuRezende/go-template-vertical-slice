package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"users-api/internal/httpapi"
	"users-api/internal/plataform/password"
	"users-api/internal/user"
	"users-api/internal/user/usertest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_Health(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hasher, err := password.NewHasher("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	h := user.NewHandler(user.NewService(usertest.NewFakeRepository(), hasher))

	w := httptest.NewRecorder()
	httpapi.NewRouter(h).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}
