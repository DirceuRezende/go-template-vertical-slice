package user

import (
	"errors"
	"net/http"
	"uuid"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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

type UserResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Age   int       `json:"age"`
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	convertedId, err := uuid.Parse(id)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrInvalidID.Error()})
		return
	}

	user, err := h.service.Get(c.Request.Context(), convertedId)

	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": &UserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Age: user.Age}})
}

func (h *Handler) Create(c *gin.Context) {
	var request UserRequest

	if err := c.ShouldBind(&request); err != nil {
		if _, ok := errors.AsType[validator.ValidationErrors](err); ok {
			c.JSON(422, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.Create(c.Request.Context(), Input{Name: request.Name, Email: request.Email, Password: request.Password, Age: request.Age})

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	userResponse := &UserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Age: user.Age}

	c.JSON(http.StatusCreated, gin.H{
		"data": userResponse,
	})
}

func (h *Handler) Update(c *gin.Context) {
	var request UpdateUserRequest
	var id, err = uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrInvalidID.Error(),
		})
		return
	}

	if err := c.ShouldBind(&request); err != nil {
		if _, ok := errors.AsType[validator.ValidationErrors](err); ok {
			c.JSON(422, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.Update(c.Request.Context(), id, Input{Name: request.Name, Email: request.Email, Age: request.Age})

	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{
			"error": err.Error(),
		})
		return
	}

	userResponse := &UserResponse{ID: user.ID, Name: user.Name, Email: user.Email, Age: user.Age}

	c.JSON(http.StatusOK, gin.H{
		"data": userResponse,
	})
}

func (h *Handler) GetAll(c *gin.Context) {
	users, err := h.service.GetAll(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	response := make([]UserResponse, 0, len(users))
	for _, u := range users {
		response = append(response, UserResponse{ID: u.ID, Name: u.Name, Email: u.Email, Age: u.Age})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}

func (h *Handler) Delete(c *gin.Context) {
	var id, err = uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": ErrInvalidID.Error(),
		})
		return
	}

	err = h.service.Delete(c.Request.Context(), id)

	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}
