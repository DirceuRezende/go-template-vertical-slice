package user

import (
	"errors"
	"net/http"
	"uuid"

	"users-api/internal/plataform/authn"

	"github.com/gin-gonic/gin"
)

type UserRequest struct {
	Name     string `json:"name" binding:"required,min=2,max=100"`
	Password string `json:"password" binding:"required,min=7,max=100,strong_password"`
	Email    string `json:"email" binding:"required,email"`
	Age      int    `json:"age" binding:"required,min=1,max=150"`
}

type UpdateUserRequest struct {
	Name  string `json:"name" binding:"required,min=2,max=100"`
	Email string `json:"email" binding:"required,email"`
	Age   int    `json:"age" binding:"required,min=1,max=150"`
}

type UpdateRolesRequest struct {
	Roles []string `json:"roles" binding:"required,min=1,dive,role_name"`
}

type UserResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Age   int       `json:"age"`
	Roles []string  `json:"roles"`
}

func toResponse(u *User) UserResponse {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return UserResponse{ID: u.ID, Name: u.Name, Email: u.Email, Age: u.Age, Roles: roles}
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// statusFor mapeia erros de domínio para o status HTTP correspondente.
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrEmailTaken), errors.Is(err, ErrLastAdmin):
		return http.StatusConflict
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrInvalidRole):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}

// authorizedID lê :id da rota e confirma que o usuário logado é o dono ou admin.
// Já responde ao cliente (400/401/403) quando falha. A checagem de permissão vem
// antes da busca, para não revelar quais ids existem.
func authorizedID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidID.Error()})
		return uuid.Nil(), false
	}

	principal, ok := authn.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
		return uuid.Nil(), false
	}
	if !principal.Can(id) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrForbidden.Error()})
		return uuid.Nil(), false
	}

	return id, true
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := authorizedID(c)
	if !ok {
		return
	}

	user, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toResponse(user)})
}

func (h *Handler) Create(c *gin.Context) {
	var request UserRequest

	if err := c.ShouldBind(&request); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := h.service.Create(c.Request.Context(), Input{Name: request.Name, Email: request.Email, Password: request.Password, Age: request.Age})
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toResponse(user)})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := authorizedID(c)
	if !ok {
		return
	}

	var request UpdateUserRequest
	if err := c.ShouldBind(&request); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := h.service.Update(c.Request.Context(), id, Input{Name: request.Name, Email: request.Email, Age: request.Age})
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toResponse(user)})
}

// UpdateRoles substitui os roles do usuário (rota restrita a admin).
func (h *Handler) UpdateRoles(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidID.Error()})
		return
	}

	var request UpdateRolesRequest
	if err := c.ShouldBind(&request); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := h.service.SetRoles(c.Request.Context(), id, request.Roles)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toResponse(user)})
}

func (h *Handler) GetAll(c *gin.Context) {
	users, err := h.service.GetAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := make([]UserResponse, 0, len(users))
	for _, u := range users {
		response = append(response, toResponse(u))
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := authorizedID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
