---
name: new-migration
description: Cria uma migration SQL (golang-migrate) em db/migrations seguindo o padrão do projeto.
---

# Migration

- Arquivos em `db/migrations/`: `NNNNNN_descricao_em_snake_case.up.sql` e `.down.sql`, numeração sequencial (próxima após o maior existente; hoje `000003`).
- **Nunca edite** migration já commitada; crie outra.
- `up` idempotente quando possível (`CREATE TABLE IF NOT EXISTS`, `CREATE UNIQUE INDEX IF NOT EXISTS`); `down` desfaz exatamente o `up` (`DROP ... IF EXISTS`).
- Tabela nova: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `created_at/updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`, nome no plural.
- Unicidade/integridade por constraint ou índice **nomeado** (`<tabela>_<coluna>_unique_idx`); o repository mapeia o erro pelo nome da constraint. Para case-insensitive use `lower(coluna)`.
- Cuidado ao adicionar constraint em tabela com dados: pode falhar com duplicatas.
- Embutidas via `db/embed.go` (`migrations/*.sql`) e aplicadas por `database.MigrateUp` no startup e nos testes.
- Cubra em `internal/plataform/database/database_integration_test.go` (ex.: índice existe em `pg_indexes`).
