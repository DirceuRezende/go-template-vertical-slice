//go:build integration || e2e

// Package testutil traz helpers para testes que precisam de infraestrutura real.
package testutil

import (
	"context"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// StartPostgres sobe um Postgres descartável e devolve a URL e a função de limpeza.
func StartPostgres(ctx context.Context) (url string, terminate func(), err error) {
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("users_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", nil, err
	}
	terminate = func() { _ = testcontainers.TerminateContainer(ctr) }

	url, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		terminate()
		return "", nil, err
	}
	return url, terminate, nil
}
