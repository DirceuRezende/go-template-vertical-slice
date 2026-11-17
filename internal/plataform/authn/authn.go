// Package authn autentica requisições HTTP via Bearer token e expõe o usuário logado.
//
// Fica em plataform (e não em auth) para que módulos de domínio, como user,
// possam autorizar sem importar o módulo auth e criar ciclo de imports.
package authn

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"uuid"

	"users-api/internal/plataform/token"

	"github.com/gin-gonic/gin"
)

const (
	AdminRole = "admin"

	principalKey = "authn.principal"
)

var (
	ErrUnauthorized = errors.New("não autenticado")
	ErrForbidden    = errors.New("acesso negado")
)

// Principal é o usuário autenticado da requisição, com os papéis do access token.
type Principal struct {
	UserID uuid.UUID
	Roles  []string
}

func (p Principal) HasRole(role string) bool { return slices.Contains(p.Roles, role) }

func (p Principal) IsAdmin() bool { return p.HasRole(AdminRole) }

// Can informa se o principal pode agir sobre o recurso do usuário informado:
// o próprio usuário ou um admin.
func (p Principal) Can(userID uuid.UUID) bool { return p.UserID == userID || p.IsAdmin() }

// Parser valida um access token.
type Parser interface {
	Parse(raw string) (token.Claims, error)
}

// Middleware exige um access token válido (assinatura e expiração). Não consulta
// o banco: os papéis vêm do próprio token.
func Middleware(parser Parser) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearer(c.GetHeader("Authorization"))
		if !ok {
			abort(c, http.StatusUnauthorized, ErrUnauthorized)
			return
		}

		claims, err := parser.Parse(raw)
		if err != nil {
			abort(c, http.StatusUnauthorized, ErrUnauthorized)
			return
		}

		c.Set(principalKey, Principal{UserID: claims.UserID, Roles: claims.Roles})
		c.Next()
	}
}

// RequireRole exige que o usuário autenticado tenha o papel. Use após Middleware.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := FromContext(c)
		if !ok {
			abort(c, http.StatusUnauthorized, ErrUnauthorized)
			return
		}
		if !p.HasRole(role) {
			abort(c, http.StatusForbidden, ErrForbidden)
			return
		}
		c.Next()
	}
}

// FromContext devolve o principal injetado pelo Middleware.
func FromContext(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}

func bearer(header string) (string, bool) {
	scheme, raw, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(raw) == "" {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

func abort(c *gin.Context, status int, err error) {
	c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
}
