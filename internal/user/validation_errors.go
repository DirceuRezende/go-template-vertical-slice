package user

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// FieldError descreve a falha de validação de um campo da requisição.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func fieldMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "campo obrigatório"
	case "email":
		return "deve ser um email válido"
	case "min":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("deve ter no mínimo %s caracteres", fe.Param())
		}
		return fmt.Sprintf("deve ser no mínimo %s", fe.Param())
	case "max":
		if fe.Kind().String() == "string" {
			return fmt.Sprintf("deve ter no máximo %s caracteres", fe.Param())
		}
		return fmt.Sprintf("deve ser no máximo %s", fe.Param())
	case "strong_password":
		return "deve conter ao menos uma letra maiúscula, uma minúscula e um símbolo (" + passwordSymbols + ")"
	case "role_name":
		return "deve ter de 2 a 32 caracteres: letras minúsculas, números e _"
	default:
		return "valor inválido (" + fe.Tag() + ")"
	}
}

// fieldErrors converte os erros do validator em uma lista legível.
func fieldErrors(errs validator.ValidationErrors) []FieldError {
	out := make([]FieldError, 0, len(errs))
	for _, fe := range errs {
		out = append(out, FieldError{Field: fe.Field(), Message: fieldMessage(fe)})
	}
	return out
}

// RespondBindError responde a falha de ShouldBind: 422 com os campos inválidos
// ou 400 quando o corpo nem é um JSON válido.
func RespondBindError(c *gin.Context, err error) {
	if verrs, ok := errors.AsType[validator.ValidationErrors](err); ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":  "dados inválidos",
			"fields": fieldErrors(verrs),
		})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "corpo da requisição inválido: " + strings.TrimSpace(err.Error())})
}
