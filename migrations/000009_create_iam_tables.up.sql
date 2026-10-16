-- IAM tables: AWS-style policies, roles, groups, attachments, and permission
-- boundaries. This supersedes the RBAC schema created in 000003; 000010
-- migrates the RBAC data into these tables and drops the old ones.
--
-- Concepts:
--   iam_policies            a named, reusable JSON policy document (validated
--                           in Go before storage): Effect(Action, Resource).
--   iam_policy_versions     immutable document history (least-privilege
--                           tightening + rollback). One version is default.
--   iam_groups              a collection of users.
--   iam_roles               an assumable identity: attached policies + an
--                           optional assume-role trust policy.
--   iam_policy_attachments  attach a policy to a user, group, or role.
--   iam_role_assignments    grant a role to a user or group.
--
-- Access is deny-by-default and an explicit Deny overrides any Allow (see
-- internal/iam).

CREATE TABLE iam_policies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    document    JSONB NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    is_builtin  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE iam_policy_versions (
    policy_id  UUID NOT NULL REFERENCES iam_policies(id) ON DELETE CASCADE,
    version    INTEGER NOT NULL,
    document   JSONB NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (policy_id, version)
);

CREATE TABLE iam_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE iam_roles (
    id                             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                           TEXT NOT NULL UNIQUE,
    description                    TEXT,
    assume_role_policy_document    JSONB,
    permissions_boundary_policy_id UUID REFERENCES iam_policies(id) ON DELETE SET NULL,
    created_at                     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE iam_policy_attachments (
    policy_id      UUID NOT NULL REFERENCES iam_policies(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'group', 'role')),
    principal_id   UUID NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (policy_id, principal_type, principal_id)
);

CREATE TABLE iam_group_memberships (
    group_id   UUID NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE iam_role_assignments (
    role_id        UUID NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'group')),
    principal_id   UUID NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, principal_type, principal_id)
);

-- A permission boundary is a per-identity ceiling: it can only restrict, never
-- grant. Stored on the user; roles carry their own boundary column.
ALTER TABLE users
    ADD COLUMN permissions_boundary_policy_id UUID
    REFERENCES iam_policies(id) ON DELETE SET NULL;

CREATE INDEX idx_iam_attachments_principal ON iam_policy_attachments (principal_type, principal_id);
CREATE INDEX idx_iam_group_memberships_user ON iam_group_memberships (user_id);
CREATE INDEX idx_iam_role_assignments_principal ON iam_role_assignments (principal_type, principal_id);
