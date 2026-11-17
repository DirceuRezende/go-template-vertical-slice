package password_test

import (
	"strings"
	"testing"

	"users-api/internal/plataform/password"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pepper = "0123456789abcdef0123456789abcdef"

func TestNewHasher_WeakPepper(t *testing.T) {
	h, err := password.NewHasher("curto")
	assert.ErrorIs(t, err, password.ErrWeakPepper)
	assert.Nil(t, h)
}

func TestHasher_HashAndMatches(t *testing.T) {
	h, err := password.NewHasher(pepper)
	require.NoError(t, err)

	hash, err := h.Hash("Senha@123")
	require.NoError(t, err)

	assert.NotEqual(t, "Senha@123", hash)
	assert.True(t, h.Matches(hash, "Senha@123"))
	assert.False(t, h.Matches(hash, "outra"))
}

func TestHasher_SaltMakesHashesDifferent(t *testing.T) {
	h, _ := password.NewHasher(pepper)

	a, err := h.Hash("Senha@123")
	require.NoError(t, err)
	b, err := h.Hash("Senha@123")
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
}

func TestHasher_DifferentPepperDoesNotMatch(t *testing.T) {
	h1, _ := password.NewHasher(pepper)
	h2, _ := password.NewHasher(strings.Repeat("x", 32))

	hash, err := h1.Hash("Senha@123")
	require.NoError(t, err)

	assert.False(t, h2.Matches(hash, "Senha@123"))
}

func TestHasher_LongPassword(t *testing.T) {
	h, _ := password.NewHasher(pepper)
	long := strings.Repeat("a", 200)

	hash, err := h.Hash(long)
	require.NoError(t, err)
	assert.True(t, h.Matches(hash, long))
	assert.False(t, h.Matches(hash, long+"b"))
}
