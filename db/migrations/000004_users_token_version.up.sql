-- Avança a cada troca de senha; refresh tokens de outra versão deixam de valer.
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INTEGER NOT NULL DEFAULT 0;
