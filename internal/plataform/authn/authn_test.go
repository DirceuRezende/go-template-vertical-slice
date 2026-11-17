package authn_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"users-api/internal/plataform/authn"
	"users-api/internal/plataform/token"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "0123456789abcdef0123456789abcdef"

func setup(t *testing.T) (*gin.Engine, *token.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	m, err := token.NewManager(secret, time.Minute)
	require.NoError(t, err)

	r := gin.New()
	r.Use(authn.Middleware(m))
	r.GET("/me", func(c *gin.Context) {
		p, _ := authn.FromContext(c)
		c.JSON(http.StatusOK, gin.H{"id": p.UserID, "roles": p.Roles})
	})
	r.GET("/admin", authn.RequireRole(authn.AdminRole), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return r, m
}

func get(r http.Handler, path, header string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMiddleware_Valid(t *testing.T) {
	id := uuid.NewV7()
	r, m := setup(t)
	raw, err := m.Issue(id, []string{"user"})
	require.NoError(t, err)

	w := get(r, "/me", "Bearer "+raw)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), id.String())
	assert.Contains(t, w.Body.String(), `"roles":["user"]`)
}

func TestMiddleware_Unauthorized(t *testing.T) {
	r, _ := setup(t)

	tests := map[string]string{
		"sem header":     "",
		"esquema errado": "Basic abc",
		"sem token":      "Bearer ",
		"token inválido": "Bearer abc.def.ghi",
	}
	for name, header := range tests {
		t.Run(name, func(t *testing.T) {
			w := get(r, "/me", header)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), `"error"`)
		})
	}
}

func TestMiddleware_TokenExpirado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m, err := token.NewManager(secret, time.Millisecond)
	require.NoError(t, err)
	r := gin.New()
	r.Use(authn.Middleware(m))
	r.GET("/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	raw, err := m.Issue(uuid.NewV7(), nil)
	require.NoError(t, err)

	time.Sleep(1100 * time.Millisecond) // exp tem precisão de segundos

	assert.Equal(t, http.StatusUnauthorized, get(r, "/me", "Bearer "+raw).Code)
}

func TestRequireRole(t *testing.T) {
	r, m := setup(t)
	adminTok, _ := m.Issue(uuid.NewV7(), []string{"user", "admin"})
	regularTok, _ := m.Issue(uuid.NewV7(), []string{"user"})

	assert.Equal(t, http.StatusNoContent, get(r, "/admin", "Bearer "+adminTok).Code)
	assert.Equal(t, http.StatusForbidden, get(r, "/admin", "Bearer "+regularTok).Code)
	assert.Equal(t, http.StatusUnauthorized, get(r, "/admin", "").Code)
}

func TestPrincipal_Can(t *testing.T) {
	self, other := uuid.NewV7(), uuid.NewV7()

	assert.True(t, authn.Principal{UserID: self}.Can(self))
	assert.False(t, authn.Principal{UserID: self}.Can(other))
	assert.True(t, authn.Principal{UserID: self, Roles: []string{"admin"}}.Can(other))
}
