// Package token emite e valida os access tokens (JWT HS256).
//
// O token é stateless: carrega sub, papéis e iat/exp, e é validado só pela
// assinatura, sem consulta ao banco. Papéis e senha alterados valem no próximo
// refresh; um access token já emitido vale até expirar.
package token

import (
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

const minSecretLen = 32

var (
	ErrWeakSecret   = errors.New("o segredo do JWT deve ter pelo menos 32 caracteres")
	ErrInvalidToken = errors.New("token inválido")
)

type claims struct {
	jwt.RegisteredClaims
	Roles []string `json:"roles"`
}

type Claims struct {
	UserID uuid.UUID
	Roles  []string
}

type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret string, accessTTL time.Duration) (*Manager, error) {
	if len(secret) < minSecretLen {
		return nil, ErrWeakSecret
	}
	if accessTTL <= 0 {
		return nil, errors.New("a duração do access token deve ser positiva")
	}
	return &Manager{secret: []byte(secret), ttl: accessTTL, now: time.Now}, nil
}

// TTL devolve a validade do access token (para o campo expires_in).
func (m *Manager) TTL() time.Duration { return m.ttl }

// Issue emite um access token para o usuário com os papéis informados.
func (m *Manager) Issue(userID uuid.UUID, roles []string) (string, error) {
	if roles == nil {
		roles = []string{}
	}
	now := m.now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
		Roles: roles,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
}

// Parse valida assinatura (somente HS256), exp e subject.
func (m *Manager) Parse(raw string) (Claims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: id, Roles: c.Roles}, nil
}
