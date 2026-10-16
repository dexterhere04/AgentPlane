-- Best-effort rollback of 000010: recreate the legacy RBAC schema and
-- repopulate the built-in roles and user assignments.
--
-- This is inherently lossy. The IAM model (explicit Deny, wildcards, groups,
-- permission boundaries, resource scoping) cannot round-trip into flat
-- permission strings. Built-in roles and user grants are restored; custom IAM
-- policies are not translated back.

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);

INSERT INTO roles (name, description) VALUES
    ('admin',  'Full access to every gateway resource'),
    ('member', 'Standard access: invoke chat and use any model')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission)
SELECT id, '*' FROM roles WHERE name = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission
FROM roles r
CROSS JOIN (VALUES ('chat:invoke'), ('model:*')) AS p(permission)
WHERE r.name = 'member'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT ra.principal_id, r.id
FROM iam_role_assignments ra
JOIN iam_roles ir ON ir.id = ra.role_id
JOIN roles r ON r.name = ir.name
WHERE ra.principal_type = 'user'
ON CONFLICT DO NOTHING;
