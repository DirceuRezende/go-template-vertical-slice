# users-api

API REST em Go (Gin + pgx + PostgreSQL). Módulo `users-api`, Go 1.27. Comentários, mensagens de erro e testes em **português**.

## Estrutura

```
cmd/api/
  main.go            # wiring: setup → migrate → pool → repo → service → handler → router → shutdown
  config.go          # config + setup(): lê env (.env opcional), cria Hasher, registra validações
db/
  embed.go           # embute migrations/*.sql
  migrations/        # NNNNNN_nome.{up,down}.sql (golang-migrate, aplicadas no startup)
internal/
  httpapi/router.go  # gin.Engine, /health e RegisterRoutes de cada módulo
  plataform/         # (sic) infra compartilhada: database (pool, migrate), password (bcrypt+pepper)
  testutil/          # StartPostgres (testcontainers) — tags integration||e2e
  user/              # módulo de domínio (modelo de referência)
    user.go            # entidade, erros de domínio, interface Repository
    service.go         # regras de negócio (Input, Service)
    handler.go         # handlers Gin, Request/Response DTOs, statusFor
    routes.go          # (h *Handler) RegisterRoutes
    validation*.go     # validações customizadas e formatação de erros de validação
    postgres/          # implementação pgx do Repository
    usertest/          # FakeRepository (in-memory) e MockRepository (testify)
test/e2e/            # testes ponta a ponta (tag e2e)
```

Dependências fluem num só sentido: `handler → service → Repository (interface em user/) ← postgres/`. O pacote de domínio **nunca** importa `postgres/`.

## Convenções

- **Módulo = pacote em `internal/<nome>/`** com: entidade + erros + `Repository` (interface), `Service`, `Handler`, `routes.go`, subpacote `postgres/`, subpacote `<nome>test/` com fake e mock.
- **IDs**: `uuid` da stdlib (`uuid.NewV7()`, `uuid.Parse`, `uuid.Nil()`); no SQL passe `id.String()` e leia com `id::text`.
- **Erros de domínio** são variáveis `Err...` no pacote do módulo; o repositório traduz erros do pg (ex.: `no rows` → `ErrNotFound`, `23505` → `Err...Taken`); o handler traduz para HTTP via `statusFor`.
- **Respostas**: sucesso `{"data": ...}`; erro `{"error": "..."}`; validação 422 `{"error":"dados inválidos","fields":[{field,message}]}`; JSON malformado 400; não encontrado 404; conflito 409; delete 204. Nunca devolver `password`.
- **Validação**: tags `binding:"..."` nos DTOs; nomes dos campos vêm do json tag; mensagens em `validation_errors.go`. Novas tags custom: registrar em `RegisterValidations` e mapear a mensagem em `fieldMessage`.
- **Banco**: unicidade/integridade sempre por constraint/índice no banco (não só checagem prévia). Migrations nunca editadas depois de commitadas — crie uma nova.
- Sem comentários óbvios; comente o *porquê* (ver `password.go`, `pool.go`).

## Comandos

```
make test              # unit + handler (sem Docker)
make test-integration  # -tags=integration (testcontainers → precisa de Docker)
make test-e2e          # -tags=e2e
make test-all          # tudo com -race
gofmt -l . && go vet -tags='integration e2e' ./...
docker compose up -d   # Postgres local; copie .env.example para .env (gere PASSWORD_PEPPER ≥ 32 chars)
go run ./cmd/api
```

## Testes (padrão)

| Nível | Onde | Doubles | Tag |
|---|---|---|---|
| Unit service | `internal/<m>/service_test.go` (`package <m>_test`) | `MockRepository` p/ interações, `FakeRepository` p/ fluxos | — |
| Handler | `internal/<m>/handler_test.go` | Service real + `FakeRepository`, `httptest` | — |
| Integração | `internal/<m>/postgres/repository_integration_test.go` | Postgres real (`testutil.StartPostgres`, `TestMain`, `TRUNCATE` por teste) | `integration` |
| E2E | `test/e2e/` | API completa + Postgres | `e2e` |

Usar `testify`: `require` para pré-condições, `assert` para verificações. Fakes devem espelhar o comportamento do Postgres (not found, unicidade).

## Skills do projeto (`.claude/skills/` p/ Claude Code, `.agents/skills/` p/ Codex — mantenha as duas cópias iguais)

`new-module` (módulo completo) · `new-endpoint` · `new-service` · `new-handler` · `new-repository` · `new-migration` · `new-tests`. Siga-as ao criar código novo para manter o padrão do módulo `user`.

## Pontos de atenção

- `PASSWORD_PEPPER` ≥ 32 chars; trocar o pepper invalida todos os hashes existentes.
- Registre validações (`user.RegisterValidations()`) antes de criar o router.
- `go get` pode ser lento por causa das dependências do testcontainers.
