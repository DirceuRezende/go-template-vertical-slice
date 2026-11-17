CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Todos os tokens de uma mesma sessão (cadeia de rotações) compartilham a família.
    family_id   UUID        NOT NULL,
    -- sha256 hex do token; o valor em claro nunca é guardado.
    token_hash  TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    -- users.token_version no momento da emissão; diferente da atual = token inválido.
    token_version INTEGER   NOT NULL,
    used_at     TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS refresh_tokens_token_hash_idx ON refresh_tokens (token_hash);
CREATE INDEX IF NOT EXISTS refresh_tokens_user_id_idx ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_family_id_idx ON refresh_tokens (family_id);
