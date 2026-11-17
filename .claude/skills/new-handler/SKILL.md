---
name: new-handler
description: Cria ou estende handlers Gin, DTOs de request/response e rotas seguindo internal/user/handler.go e routes.go.
---

# Handler

Modelo: `internal/user/handler.go`, `routes.go`, `validation_errors.go`.

- DTOs: `<Entidade>Request` (create) e `Update<Entidade>Request` (sem campos imutáveis, ex.: senha), com `json` e `binding` tags (`required,min=,max=,email`...). `<Entidade>Response` **sem** campos sensíveis.
- `Handler{service *Service}` + `NewHandler`.
- Rotas em `routes.go`: `func (h *Handler) RegisterRoutes(r gin.IRouter, authenticate gin.HandlerFunc)` com `g := r.Group("/<plural>", authenticate)` (rotas protegidas por padrão; deixe pública só o que for intencional, como `POST /users`): `GET ""`, `POST ""`, `GET/PUT/DELETE "/:id"`.
- Fluxo de cada handler:
  1. `uuid.Parse(c.Param("id"))` → falha: 400 `{"error": ErrInvalidID.Error()}`.
  2. `c.ShouldBind(&req)` → falha: `respondBindError(c, err)` (422 com `fields`, ou 400 p/ JSON malformado).
  3. chama o service com `c.Request.Context()`.
  4. erro → `c.JSON(statusFor(err), gin.H{"error": err.Error()})`.
  5. sucesso → `gin.H{"data": ...}`; criação 201; delete `c.Status(http.StatusNoContent)`; listagem nunca `null` (`make([]T, 0, n)`).
- `statusFor`: `ErrNotFound`→404, `Err...Taken`→409, default→400 (adicione novos erros de domínio aqui).
- Nova tag de validação: registre em `RegisterValidations` (validation.go) e adicione a mensagem em `fieldMessage` (validation_errors.go) + caso em `validation_test.go`.
- Registre o handler em `internal/httpapi/router.go`.
