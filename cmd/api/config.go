package main

import (
	"errors"
	"fmt"
	"os"

	"users-api/internal/plataform/password"
	"users-api/internal/user"

	"github.com/joho/godotenv"
)

type config struct {
	databaseURL string
	port        string
	hasher      *password.Hasher
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Precisa vir antes de subir o router: registra a tag strong_password.
	if err := user.RegisterValidations(); err != nil {
		return config{}, fmt.Errorf("register validations: %w", err)
	}

	return config{databaseURL: databaseURL, port: port, hasher: hasher}, nil
}
