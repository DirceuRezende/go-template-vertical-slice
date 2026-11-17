package user

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrongPassword(t *testing.T) {
	v := validator.New()
	require.NoError(t, v.RegisterValidation("strong_password", strongPassword))

	tests := []struct {
		name  string
		input string
		ok    bool
	}{
		{"completa com @", "Abcdef@1", true},
		{"completa com $", "abcDEF$x", true},
		{"completa com #", "abcDEF#x", true},
		{"completa com %", "abcDEF%x", true},
		{"completa com &", "abcDEF&x", true},
		{"sem maiúscula", "abcdef@1", false},
		{"sem minúscula", "ABCDEF@1", false},
		{"sem símbolo", "Abcdefg1", false},
		{"símbolo não permitido", "Abcdef!1", false},
		{"vazia", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Var(tt.input, "strong_password")
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestRegisterValidations(t *testing.T) {
	assert.NoError(t, RegisterValidations())
}
