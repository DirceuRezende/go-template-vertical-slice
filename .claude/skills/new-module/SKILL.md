---
name: new-module
description: Cria um módulo de domínio completo (entidade, repository, service, handler, rotas, migration, testes) seguindo o padrão do módulo user. Use quando o usuário pedir um novo recurso/entidade (ex.: "crie o módulo de produtos").
---

# Novo módulo

Modelo de referência: `internal/user/`. Substitua `user` pelo nome no singular, minúsculo (ex.: `product`).

Ordem (cada passo tem sua skill detalhada):

1. **Migration** → skill `new-migration` (tabela `<plural>`).
2. **Entidade, erros e interface** em `internal/<m>/<m>.go`: struct, `ErrNotFound`/`ErrInvalidID` (+ erros específicos), `Repository` com `Save/Update/Delete/FindOne/FindAll(ctx, ...)`.
3. **Repository Postgres** → skill `new-repository` (`internal/<m>/postgres/repository.go`).
4. **Service** → skill `new-service`.
5. **Handler + DTOs + `routes.go`** → skill `new-handler` / `new-endpoint`.
6. **Doubles** `internal/<m>/<m>test/`: copie `usertest/fake_repository.go` e `mock_repository.go` adaptando tipos e regras de unicidade.
7. **Wiring**: em `internal/httpapi/router.go` receba o novo handler em `NewRouter(...)` e chame `RegisterRoutes(router)`; em `cmd/api/main.go` monte `postgres.NewRepository(pool)` → `NewService` → `NewHandler` e passe ao router. Atualize `router_test.go` e o e2e (`TestMain`).
8. **Testes** → skill `new-tests` (unit, handler, integração, e2e).
9. Atualize a árvore em `CLAUDE.md` se surgir algo novo.

Termine com `gofmt -l .`, `go vet -tags='integration e2e' ./...` e `make test-all`.
