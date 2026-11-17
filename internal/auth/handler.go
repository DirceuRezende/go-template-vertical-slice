package auth

import (
	"errors"
	"log"
	"net/http"

	"users-api/internal/plataform/authn"
	"users-api/internal/user"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=7,max=100,strong_password"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

func toTokenResponse(p *TokenPair) TokenResponse {
	return TokenResponse{
		AccessToken: p.AccessToken, RefreshToken: p.RefreshToken,
		TokenType: "Bearer", ExpiresIn: int(p.ExpiresIn.Seconds()),
	}
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// respondError mapeia erros de domínio; o que não é conhecido vira 500 genérico
// (o detalhe vai para o log, não para o cliente).
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidRefreshToken):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, user.ErrNotFound):
		// Usuário excluído entre a autenticação e a operação.
		c.JSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
	case errors.Is(err, ErrWrongPassword):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		log.Printf("auth: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro interno"})
	}
}

func (h *Handler) Login(c *gin.Context) {
	var request LoginRequest
	if err := c.ShouldBind(&request); err != nil {
		user.RespondBindError(c, err)
		return
	}

	pair, err := h.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toTokenResponse(pair)})
}

func (h *Handler) Refresh(c *gin.Context) {
	var request RefreshRequest
	if err := c.ShouldBind(&request); err != nil {
		user.RespondBindError(c, err)
		return
	}

	pair, err := h.service.Refresh(c.Request.Context(), request.RefreshToken)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toTokenResponse(pair)})
}

func (h *Handler) Logout(c *gin.Context) {
	var request RefreshRequest
	if err := c.ShouldBind(&request); err != nil {
		user.RespondBindError(c, err)
		return
	}

	if err := h.service.Logout(c.Request.Context(), request.RefreshToken); err != nil {
		respondError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *Handler) ChangePassword(c *gin.Context) {
	principal, ok := authn.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
		return
	}

	var request ChangePasswordRequest
	if err := c.ShouldBind(&request); err != nil {
		user.RespondBindError(c, err)
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), principal.UserID, request.CurrentPassword, request.NewPassword); err != nil {
		respondError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
