# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.3-alpine AS build
WORKDIR /src

# Dependências primeiro: esta camada só é refeita quando go.mod/go.sum mudam.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Binário estático (sem CGO), sem símbolos de debug e sem paths locais.
# As migrations vão embutidas no binário via db/embed.go.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime ----
# distroless/static: sem shell nem gerenciador de pacotes, já com CA certs e usuário não-root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api

ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/api"]
