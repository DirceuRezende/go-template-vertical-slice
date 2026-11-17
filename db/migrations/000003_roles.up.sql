CREATE TABLE IF NOT EXISTS roles (
    name TEXT PRIMARY KEY CHECK (name ~ '^[a-z0-9_]{2,32}$')
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role    TEXT NOT NULL REFERENCES roles (name),
    PRIMARY KEY (user_id, role)
);

-- Consulta "quantos admins restam" e remoção de role.
CREATE INDEX IF NOT EXISTS user_roles_role_idx ON user_roles (role);

INSERT INTO roles (name) VALUES ('user'), ('admin') ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role)
SELECT id, 'user' FROM users
ON CONFLICT DO NOTHING;
