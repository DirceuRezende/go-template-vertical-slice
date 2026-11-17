package token

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "0123456789abcdef0123456789abcdef"

func newManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(secret, 15*time.Minute)
	require.NoError(t, err)
	return m
}

func TestNewManager_Validations(t *testing.T) {
	_, err := NewManager("curto", time.Minute)
	assert.ErrorIs(t, err, ErrWeakSecret)

	_, err = NewManager(secret, 0)
	assert.Error(t, err)
}

func TestManager_IssueAndParse(t *testing.T) {
	m := newManager(t)
	id := uuid.NewV7()

	raw, err := m.Issue(id, []string{"user", "admin"})
	require.NoError(t, err)

	claims, err := m.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, id, claims.UserID)
	assert.Equal(t, []string{"user", "admin"}, claims.Roles)
}

func TestManager_Issue_SemRolesGeraListaVazia(t *testing.T) {
	m := newManager(t)

	raw, err := m.Issue(uuid.NewV7(), nil)
	require.NoError(t, err)

	claims, err := m.Parse(raw)
	require.NoError(t, err)
	assert.Empty(t, claims.Roles)
}

func TestManager_Parse_Expired(t *testing.T) {
	m := newManager(t)
	raw, err := m.Issue(uuid.NewV7(), nil)
	require.NoError(t, err)

	m.now = func() time.Time { return time.Now().Add(16 * time.Minute) }

	_, err = m.Parse(raw)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestManager_Parse_Invalid(t *testing.T) {
	m := newManager(t)
	other, err := NewManager("ffffffffffffffffffffffffffffffff", time.Minute)
	require.NoError(t, err)
	foreign, err := other.Issue(uuid.NewV7(), nil)
	require.NoError(t, err)

	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject:   uuid.NewV7().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.RegisteredClaims{
		Subject:   uuid.NewV7().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	noExp, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  uuid.NewV7().String(),
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	badSub, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "não-é-uuid",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte(secret))
	require.NoError(t, err)

	good, err := m.Issue(uuid.NewV7(), nil)
	require.NoError(t, err)

	tests := map[string]string{
		"vazio":              "",
		"lixo":               "abc.def.ghi",
		"assinatura errada":  foreign,
		"alg none":           none,
		"alg diferente":      hs512,
		"sem exp":            noExp,
		"subject inválido":   badSub,
		"payload adulterado": good[:strings.LastIndex(good, ".")] + ".assinatura",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := m.Parse(raw)
			assert.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}
