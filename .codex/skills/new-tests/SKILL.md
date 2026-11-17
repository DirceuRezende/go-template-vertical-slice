---
name: new-tests
description: Escreve testes unitários, de handler, de integração (testcontainers) e e2e seguindo o padrão do projeto (testify, mock, fake).
---

# Testes

Siga os arquivos existentes como modelo. Pacotes de teste são externos (`package user_test`) exceto quando precisam de símbolos privados (`validation_test.go`).

**Unit — service** (`service_test.go`): `usertest.MockRepository` com `repo.On(...).Return(...)`, `mock.Anything`/`mock.MatchedBy`, `AssertExpectations`, `AssertNotCalled`. Um fluxo completo com `FakeRepository`. Helper `newHasher(t)` (pepper de 32 chars).

**Handler** (`handler_test.go`): `gin.SetMode(gin.TestMode)`, `RegisterValidations()`, Service real + `FakeRepository`, helpers `newRouter`/`do`/`create`/`createWithEmail`, `httptest`. Cubra: sucesso, 400 (id/JSON), 404, 409, 422 (com `assert.JSONEq` do corpo de `fields`), 204, listagem vazia `{"data":[]}`, ausência de campos sensíveis.

**Integração** (`postgres/repository_integration_test.go`, `//go:build integration`): `TestMain` com `testutil.StartPostgres` + `database.MigrateUp` + `database.NewPool`; `newRepo(t)` faz `TRUNCATE`. Cubra CRUD, not found, ordenação, lista vazia não-nil, cada constraint (ex.: email duplicado case-insensitive → erro de domínio; PK duplicada → erro genérico).

**E2E** (`test/e2e/`, `//go:build e2e`): sobe o container, monta o mesmo wiring do `main`, `httptest.NewServer`, client `net/http`; helpers `call`/`createX`/`reset`. Cubra CRUD completo, erros 4xx, e que dados sensíveis (senha) são hasheados no banco e nunca expostos.

**Doubles**: ao mudar a interface `Repository` ou regras do banco, atualize `FakeRepository` (mesmo comportamento do Postgres: not found, unicidade case-insensitive) e `MockRepository`.

Execução: `make test`, `make test-integration`, `make test-e2e`, `make test-all` (usa `-race`). Antes de concluir: `gofmt -l .` e `go vet -tags='integration e2e' ./...` limpos.
