package user

import "github.com/gin-gonic/gin"

// RegisterRoutes registra as rotas do módulo de usuários.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	g := r.Group("/users")
	g.GET("", h.GetAll)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}
