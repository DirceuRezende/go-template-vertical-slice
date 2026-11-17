---
name: new-repository
description: Cria ou estende um repository PostgreSQL (pgx) seguindo internal/user/postgres/repository.go.
---

# Repository Postgres

Modelo: `internal/user/postgres/repository.go`. Pacote `postgres` dentro do módulo; importa o pacote de domínio (nunca o contrário).

- `type Repository struct{ pool *pgxpool.Pool }` + `NewRepository(pool)`; implementa a interface `Repository` do domínio (`var _ user.Repository = (*Repository)(nil)` é bem-vindo).
- Colunas numa constante (`userColumns = "id::text, name, ..."`) reutilizada em `RETURNING`/`SELECT`; `scanUser(row pgx.Row)` único para `QueryRow` e `rows`.
- SQL parametrizado (`$1...`), ids como `id.String()`.
- `pgx.ErrNoRows` → `ErrNotFound`; `Delete` com `RowsAffected()==0` → `ErrNotFound`.
- Violações de constraint → erro de domínio via `mapError` (`errors.AsType[*pgconn.PgError]`, `pgerrcode.UniqueViolation` **e** `ConstraintName` específico; outras violações seguem como erro genérico).
- `FindAll` devolve slice não-nil (`[]*T{}`), com `ORDER BY` determinístico e `rows.Err()`.
- `Update` nunca altera campos imutáveis (senha, `created_at`); atualiza `updated_at = now()`.
- Mudança de schema → migration nova (skill `new-migration`). Se mudar regra de unicidade, atualize o `FakeRepository`.
- Teste de integração obrigatório (skill `new-tests`).
