// Package httpapi monta o router HTTP da aplicação.
package httpapi

import (
	"net/http"

	"users-api/internal/auth"
	"users-api/internal/plataform/authn"
	"users-api/internal/user"

	"github.com/gin-gonic/gin"
)

// NewRouter cria o engine do Gin e registra as rotas de cada módulo.
// Ao criar um módulo novo, receba o handler aqui e chame o seu RegisterRoutes.
// parser alimenta o middleware que protege as rotas autenticadas.
func NewRouter(userHandler *user.Handler, authHandler *auth.Handler, parser authn.Parser) *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	authenticate := authn.Middleware(parser)
	userHandler.RegisterRoutes(router, authenticate)
	authHandler.RegisterRoutes(router, authenticate)

	return router
}
