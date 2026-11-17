package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"
	"uuid"

	"users-api/internal/plataform/password"
	"users-api/internal/user"
)

// TokenPair é o resultado de login e refresh.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

type Service struct {
	repo       Repository
	users      Users
	hasher     *password.Hasher
	access     AccessTokens
	refreshTTL time.Duration

	// dummyHash iguala o custo de um login com e-mail inexistente ao de um com
	// senha errada, para não revelar por tempo de resposta quais e-mails existem.
	dummyHash func() string
}

func NewService(repo Repository, users Users, hasher *password.Hasher, access AccessTokens, refreshTTL time.Duration) *Service {
	return &Service{
		repo: repo, users: users, hasher: hasher, access: access, refreshTTL: refreshTTL,
		dummyHash: sync.OnceValue(func() string {
			h, _ := hasher.Hash("senha-fictícia")
			return h
		}),
	}
}

// newRefreshToken gera 32 bytes aleatórios; só o sha256 vai ao banco.
func newRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Service) pair(userID uuid.UUID, roles []string, refreshRaw string) (*TokenPair, error) {
	access, err := s.access.Issue(userID, roles)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refreshRaw, ExpiresIn: s.access.TTL()}, nil
}

// Login valida e-mail e senha e abre uma nova sessão. Qualquer falha de
// credencial devolve o mesmo erro, sem indicar qual campo falhou.
func (s *Service) Login(ctx context.Context, email, plainPassword string) (*TokenPair, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, user.ErrNotFound) {
		s.hasher.Matches(s.dummyHash(), plainPassword)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !s.hasher.Matches(u.Password, plainPassword) {
		return nil, ErrInvalidCredentials
	}

	raw, hash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	err = s.repo.Create(ctx, RefreshToken{
		ID: uuid.NewV7(), UserID: u.ID, FamilyID: uuid.NewV7(),
		TokenHash: hash, TokenVersion: u.TokenVersion, ExpiresAt: now.Add(s.refreshTTL), CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}

	return s.pair(u.ID, u.Roles, raw)
}

// Refresh troca um refresh token por um novo par. O token apresentado deixa de
// valer; reapresentá-lo revoga a sessão inteira. Os papéis do novo access token
// vêm do banco, e uma troca de senha desde a emissão (token_version) invalida a
// sessão.
func (s *Service) Refresh(ctx context.Context, presented string) (*TokenPair, error) {
	raw, hash, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()

	stored, err := s.repo.Rotate(ctx, hashToken(presented), RefreshToken{
		ID: uuid.NewV7(), TokenHash: hash, ExpiresAt: now.Add(s.refreshTTL), CreatedAt: now,
	}, now)

	if errors.Is(err, ErrTokenReuse) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}

	u, err := s.users.Get(ctx, stored.UserID)
	if errors.Is(err, user.ErrNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}
	if u.TokenVersion != stored.TokenVersion {
		// O novo token já foi gravado; revoga a sessão para ele não sobreviver.
		if _, err := s.repo.RevokeFamily(ctx, hash, now); err != nil {
			return nil, err
		}
		return nil, ErrInvalidRefreshToken
	}

	return s.pair(u.ID, u.Roles, raw)
}

// Logout encerra a sessão do refresh token. O access token já emitido vale até
// expirar. É idempotente: token desconhecido não é erro.
func (s *Service) Logout(ctx context.Context, presented string) error {
	_, err := s.repo.RevokeFamily(ctx, hashToken(presented), time.Now())
	if errors.Is(err, ErrInvalidRefreshToken) {
		return nil
	}
	return err
}

// ChangePassword troca a senha. UpdatePassword incrementa token_version, o que
// derruba todas as sessões (refresh tokens): o cliente precisa logar de novo.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	if !s.hasher.Matches(u.Password, current) {
		return ErrWrongPassword
	}

	hash, err := s.hasher.Hash(next)
	if err != nil {
		return err
	}

	return s.users.UpdatePassword(ctx, userID, hash)
}
