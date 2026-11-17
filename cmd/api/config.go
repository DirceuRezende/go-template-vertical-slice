package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"users-api/internal/plataform/password"
	"users-api/internal/plataform/token"
	"users-api/internal/user"

	"github.com/joho/godotenv"
)

type config struct {
	databaseURL string
	port        string
	hasher      *password.Hasher
	tokens      *token.Manager
	refreshTTL  time.Duration

	// Admin inicial (opcional): ambos vazios ou ambos preenchidos.
	adminEmail    string
	adminPassword string
}

// durationEnv lê uma duração (ex.: "15m", "168h") ou devolve o padrão.
func durationEnv(name string, def time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: duração inválida %q (ex.: 15m, 168h)", name, raw)
	}
	return d, nil
}

// setup carrega a configuração do ambiente e prepara dependências globais.
func setup() (config, error) {
	// .env é opcional: em produção as variáveis vêm do ambiente.
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return config{}, errors.New("DATABASE_URL não definida")
	}

	hasher, err := password.NewHasher(os.Getenv("PASSWORD_PEPPER"))
	if err != nil {
		return config{}, fmt.Errorf("PASSWORD_PEPPER: %w", err)
	}

	accessTTL, err := durationEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return config{}, err
	}
	refreshTTL, err := durationEnv("REFRESH_TOKEN_TTL", 7*24*time.Hour)
	if err != nil {
		return config{}, err
	}

	tokens, err := token.NewManager(os.Getenv("JWT_SECRET"), accessTTL)
	if err != nil {
		return config{}, fmt.Errorf("JWT_SECRET: %w", err)
	}

	adminEmail, adminPassword := os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD")
	if (adminEmail == "") != (adminPassword == "") {
		return config{}, errors.New("ADMIN_EMAIL e ADMIN_PASSWORD devem ser definidos juntos")
	}

	if adminPassword != "" && len(adminPassword) < 7 {
		return config{}, errors.New("ADMIN_PASSWORD deve ter pelo menos 7 caracteres")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Precisa vir antes de subir o router: registra a tag strong_password.
	if err := user.RegisterValidations(); err != nil {
		return config{}, fmt.Errorf("register validations: %w", err)
	}

	return config{
		databaseURL: databaseURL, port: port, hasher: hasher,
		tokens: tokens, refreshTTL: refreshTTL,
		adminEmail: adminEmail, adminPassword: adminPassword,
	}, nil
}
