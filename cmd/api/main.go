package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"users-api/internal/httpapi"
	"users-api/internal/plataform/database"
	"users-api/internal/plataform/password"
	"users-api/internal/user"

	"github.com/joho/godotenv"
)

func main() {
	// .env é opcional: em produção as variáveis vêm do ambiente.
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL não definida")
	}

	hasher, err := password.NewHasher(os.Getenv("PASSWORD_PEPPER"))
	if err != nil {
		log.Fatalf("PASSWORD_PEPPER: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Precisa vir antes de subir o router: registra a tag strong_password.
	if err := user.RegisterValidations(); err != nil {
		log.Fatalf("register validations: %v", err)
	}

	if err := database.MigrateUp(databaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, database.DefaultConfig(databaseURL, "users-api"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repository := user.NewPostgresRepository(pool)
	service := user.NewService(repository, hasher)
	userHandler := user.NewHandler(service)

	router := httpapi.NewRouter(userHandler)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
