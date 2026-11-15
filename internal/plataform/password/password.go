// Package password encapsula o hash e a verificação de senhas com bcrypt + pepper.
//
// O bcrypt gera um salt aleatório a cada hash e o embute no próprio resultado.
// Além disso, a senha passa por HMAC-SHA256 com um pepper (segredo vindo do
// ambiente, fora do banco) antes do bcrypt. Assim, um vazamento só do banco
// não basta para atacar os hashes. O HMAC também reduz qualquer senha a 44
// caracteres, contornando o limite de 72 bytes do bcrypt.
package password

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const minPepperLen = 32

var ErrWeakPepper = errors.New("o pepper deve ter pelo menos 32 caracteres")

type Hasher struct {
	pepper []byte
}

func NewHasher(pepper string) (*Hasher, error) {
	if len(pepper) < minPepperLen {
		return nil, ErrWeakPepper
	}
	return &Hasher{pepper: []byte(pepper)}, nil
}

func (h *Hasher) prehash(plain string) []byte {
	mac := hmac.New(sha256.New, h.pepper)
	mac.Write([]byte(plain))
	return []byte(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// Hash gera o hash bcrypt da senha.
func (h *Hasher) Hash(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(h.prehash(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Matches informa se a senha em texto puro corresponde ao hash.
func (h *Hasher) Matches(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), h.prehash(plain)) == nil
}
