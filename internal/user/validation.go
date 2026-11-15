package user

import (
	"strings"
	"unicode"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

const passwordSymbols = "@$#%&"

// strongPassword exige ao menos uma letra maiúscula, uma minúscula e um dos símbolos @ $ # % &.
// O tamanho (7 a 100) fica por conta das tags min/max.
func strongPassword(fl validator.FieldLevel) bool {
	var upper, lower, symbol bool

	for _, r := range fl.Field().String() {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case strings.ContainsRune(passwordSymbols, r):
			symbol = true
		}
	}

	return upper && lower && symbol
}

// RegisterValidations registra as validações customizadas no validator do Gin.
// Chame uma vez na inicialização (ex.: no main, antes de subir o router).
func RegisterValidations() error {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		return v.RegisterValidation("strong_password", strongPassword)
	}
	return nil
}
