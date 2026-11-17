---
name: new-endpoint
description: Adiciona um endpoint HTTP a um módulo existente (rota, handler, método de service, repository, testes) seguindo o padrão do módulo user.
---

# Novo endpoint em módulo existente

Percorra a pilha de baixo para cima; só altere as camadas necessárias:

1. **Repository** (se precisar de dado novo): método na interface em `internal/<m>/<m>.go` + implementação em `postgres/repository.go` (skill `new-repository`) + **fake** e **mock** em `<m>test/`.
2. **Service**: método em `service.go` (skill `new-service`).
3. **Handler**: função em `handler.go` e rota em `routes.go` (`g.GET/POST/PUT/DELETE`) — skill `new-handler`.
4. **Testes**: service (mock), handler (fake), integração do repo, e2e — skill `new-tests`.

Regras de HTTP: sucesso `{"data":...}` (201 criação, 204 delete sem corpo), erros via `statusFor(err)`, id inválido → 400 `ErrInvalidID`, validação → `respondBindError`.
Se o endpoint tiver erro de domínio novo, adicione-o em `statusFor` (ex.: 409) e cubra em handler + e2e.
