package auth

import "github.com/gin-gonic/gin"

// RegisterRoutes registra as rotas de autenticação. login, refresh e logout são
// públicos (o refresh token é a credencial); troca de senha exige access token.
func (h *Handler) RegisterRoutes(r gin.IRouter, authenticate gin.HandlerFunc) {
	g := r.Group("/auth")
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
	g.POST("/change-password", authenticate, h.ChangePassword)
}
