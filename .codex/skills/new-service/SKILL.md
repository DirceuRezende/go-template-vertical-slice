---
name: new-service
description: Cria ou estende um Service de domínio (regras de negócio) seguindo internal/user/service.go.
---

# Service

Modelo: `internal/user/service.go`.

- `type Input struct` com os dados de entrada (independente do DTO HTTP).
- `type Service struct { repo Repository; ... }` + `NewService(repository Repository, ...) *Service` — dependa da **interface** `Repository`, nunca do pacote `postgres`.
- Métodos recebem `ctx context.Context` primeiro e retornam `(*Entidade, error)`.
- Valide invariantes baratas antes de ir ao repo (ex.: `id == uuid.Nil()` → `ErrInvalidID`, sem chamar o repo).
- Gere `ID` (`uuid.NewV7()`) e timestamps no service; senhas passam por `hasher.Hash` e **nunca** são devolvidas/atualizadas em `Update`.
- Update: `Get` primeiro (propaga `ErrNotFound`), altera campos, `repo.Update`.
- Propague erros do repo sem embrulhar quando forem erros de domínio (`ErrNotFound`, `Err...Taken`) para o handler usar `errors.Is`.
- Unicidade é garantida pelo banco; não faça `SELECT` prévio.

Testes (`service_test.go`, `package <m>_test`): `MockRepository` para verificar chamadas/`AssertNotCalled`/propagação de erro; `FakeRepository` para um fluxo completo (create→get→update→delete→get not found). Ver skill `new-tests`.
