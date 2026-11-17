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

	"users-api/internal/auth"
	authpostgres "users-api/internal/auth/postgres"
	"users-api/internal/httpapi"
	"users-api/internal/plataform/database"
	"users-api/internal/user"
	"users-api/internal/user/postgres"
)

func main() {
	cfg, err := setup()
	if err != nil {
		log.Fatal(err)
	}

	if err := database.MigrateUp(cfg.databaseURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.databaseURL, "users-api"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	repository := postgres.NewRepository(pool)
	service := user.NewService(repository, cfg.hasher)
	userHandler := user.NewHandler(service)

	if cfg.adminEmail != "" {
		if err := service.EnsureAdmin(ctx, cfg.adminEmail, cfg.adminPassword); err != nil {
			log.Fatalf("admin inicial: %v", err)
		}
	}

	authService := auth.NewService(authpostgres.NewRepository(pool), service, cfg.hasher, cfg.tokens, cfg.refreshTTL)
	authHandler := auth.NewHandler(authService)

	router := httpapi.NewRouter(userHandler, authHandler, cfg.tokens)

	srv := &http.Server{
		Addr:              ":" + cfg.port,
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
