-- RBAC policy tables. Roles are assigned to users; each role grants a set
-- of permissions (opaque strings such as "chat:invoke" or "model:gpt-4o").
--
-- Access is deny-by-default: a user with no roles is granted nothing. The
-- seed below creates the built-in roles but deliberately does NOT assign
-- them to any user — an operator must grant a role explicitly.

CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);

-- Built-in roles. ON CONFLICT keeps this idempotent if the migration is
-- re-run against a database that already has these rows.
INSERT INTO roles (name, description) VALUES
    ('admin',  'Full access to every gateway resource'),
    ('member', 'Standard access: invoke chat and use any model')
ON CONFLICT (name) DO NOTHING;

-- admin: wildcard permission (matches everything).
INSERT INTO role_permissions (role_id, permission)
SELECT id, '*' FROM roles WHERE name = 'admin'
ON CONFLICT DO NOTHING;

-- member: chat invocation plus access to every model.
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM roles r
CROSS JOIN (VALUES ('chat:invoke'), ('model:*')) AS p(permission)
WHERE r.name = 'member'
ON CONFLICT DO NOTHING;
