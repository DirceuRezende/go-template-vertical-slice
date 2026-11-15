// Package httpapi monta o router HTTP da aplicação.
package httpapi

import (
	"net/http"

	"users-api/internal/user"

	"github.com/gin-gonic/gin"
)

// NewRouter cria o engine do Gin e registra as rotas de cada módulo.
// Ao criar um módulo novo, receba o handler aqui e chame o seu RegisterRoutes.
func NewRouter(userHandler *user.Handler) *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	userHandler.RegisterRoutes(router)

	return router
}
