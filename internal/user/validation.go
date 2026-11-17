package user

import (
	"reflect"
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

// roleName valida o formato de um nome de role (mesma regra do banco).
func roleName(fl validator.FieldLevel) bool { return validRoleName(fl.Field().String()) }

// RegisterValidations registra as validações customizadas no validator do Gin.
// Chame uma vez na inicialização (ex.: no main, antes de subir o router).
func RegisterValidations() error {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		// Os erros usam o nome do campo no JSON (ex.: "password"), não o da struct.
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			return name
		})
		if err := v.RegisterValidation("strong_password", strongPassword); err != nil {
			return err
		}
		return v.RegisterValidation("role_name", roleName)
	}
	return nil
}
