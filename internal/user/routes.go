package user

import (
	"users-api/internal/plataform/authn"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registra as rotas do módulo de usuários. authenticate é o
// middleware de autenticação; só o cadastro (POST /users) é público.
func (h *Handler) RegisterRoutes(r gin.IRouter, authenticate gin.HandlerFunc) {
	g := r.Group("/users")
	g.POST("", h.Create)

	protected := g.Group("", authenticate)
	protected.GET("", authn.RequireRole(authn.AdminRole), h.GetAll)
	protected.GET("/:id", h.Get)
	protected.PUT("/:id", h.Update)
	protected.DELETE("/:id", h.Delete)
	protected.PUT("/:id/roles", authn.RequireRole(authn.AdminRole), h.UpdateRoles)
}
