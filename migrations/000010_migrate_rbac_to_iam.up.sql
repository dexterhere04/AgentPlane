-- Migrate the legacy RBAC schema (roles / role_permissions / user_roles,
-- created in 000003) into the IAM tables, then drop the legacy tables.
--
-- Mapping of a legacy flat permission string to an IAM statement:
--   '*'            -> Allow ap:*        on arn:ap:*
--   'admin:*'      -> Allow ap:*        on arn:ap:*
--   'chat:invoke'  -> Allow ap:InvokeChat on *
--   'model:*'      -> Allow ap:InvokeModel on arn:ap:model/*
--   'model:<name>' -> Allow ap:InvokeModel on arn:ap:model/<name>
--   anything else  -> Allow ap:<sanitized> on arn:ap:*  (opaque passthrough)
--
-- Each legacy role becomes an IAM role with a single attached managed policy
-- built from its permissions. Legacy user->role assignments become role
-- grants. Access remains deny-by-default, so a user with no role is still
-- granted nothing.
--
-- The mapping preserves the legacy intent. Opaque permissions cannot be
-- translated faithfully and are sanitized into an action token; an operator
-- should review such policies (they are stored, not rejected, so the migration
-- never fails on custom data).

DO $$
DECLARE
    lr          RECORD;
    perm        TEXT;
    stmts       JSONB;
    stmt        JSONB;
    builtin     BOOLEAN;
    policy_name TEXT;
BEGIN
    -- Nothing to migrate if the RBAC schema is absent.
    IF to_regclass('public.roles') IS NULL THEN
        RETURN;
    END IF;

    FOR lr IN
        SELECT id, name, COALESCE(description, '') AS description
        FROM roles
        ORDER BY name
    LOOP
        builtin := lr.name IN ('admin', 'member');

        -- The built-in roles reuse the recommended policy names created by
        -- SeedBuiltins (APAdmin/APMember) so a fresh install and an upgraded
        -- install converge instead of producing two equivalent policies.
        policy_name := CASE lr.name
            WHEN 'admin'  THEN 'APAdmin'
            WHEN 'member' THEN 'APMember'
            ELSE lr.name
        END;

        INSERT INTO iam_roles (name, description)
        VALUES (lr.name, lr.description)
        ON CONFLICT (name) DO NOTHING;

        stmts := '[]'::jsonb;
        FOR perm IN
            SELECT permission
            FROM role_permissions
            WHERE role_id = lr.id
            ORDER BY permission
        LOOP
            IF perm = '*' OR perm = 'admin:*' THEN
                stmt := jsonb_build_object(
                    'Effect', 'Allow',
                    'Action', jsonb_build_array('ap:*'),
                    'Resource', jsonb_build_array('arn:ap:*'));
            ELSIF perm = 'chat:invoke' THEN
                stmt := jsonb_build_object(
                    'Effect', 'Allow',
                    'Action', jsonb_build_array('ap:InvokeChat'),
                    'Resource', jsonb_build_array('*'));
            ELSIF perm = 'model:*' THEN
                stmt := jsonb_build_object(
                    'Effect', 'Allow',
                    'Action', jsonb_build_array('ap:InvokeModel'),
                    'Resource', jsonb_build_array('arn:ap:model/*'));
            ELSIF perm LIKE 'model:%' THEN
                stmt := jsonb_build_object(
                    'Effect', 'Allow',
                    'Action', jsonb_build_array('ap:InvokeModel'),
                    'Resource', jsonb_build_array('arn:ap:model/' || substr(perm, 7)));
            ELSE
                stmt := jsonb_build_object(
                    'Effect', 'Allow',
                    'Action', jsonb_build_array('ap:' || regexp_replace(perm, '[^A-Za-z0-9_]', '_', 'g')),
                    'Resource', jsonb_build_array('arn:ap:*'));
            END IF;
            stmts := stmts || jsonb_build_array(stmt);
        END LOOP;

        -- A policy document must contain at least one statement; skip roles
        -- that had no permissions (they still get an IAM role, empty of policy).
        IF jsonb_array_length(stmts) > 0 THEN
            INSERT INTO iam_policies (name, description, document, is_builtin)
            VALUES (
                policy_name,
                lr.description,
                jsonb_build_object('Version', '2026-01-01', 'Statement', stmts),
                builtin)
            ON CONFLICT (name) DO NOTHING;

            INSERT INTO iam_policy_attachments (policy_id, principal_type, principal_id)
            SELECT p.id, 'role', ro.id
            FROM iam_policies p
            JOIN iam_roles ro ON ro.name = lr.name
            WHERE p.name = policy_name
            ON CONFLICT DO NOTHING;
        END IF;
    END LOOP;

    -- Legacy user->role assignments become role grants to users.
    INSERT INTO iam_role_assignments (role_id, principal_type, principal_id)
    SELECT ro.id, 'user', ur.user_id
    FROM user_roles ur
    JOIN roles r ON r.id = ur.role_id
    JOIN iam_roles ro ON ro.name = r.name
    ON CONFLICT DO NOTHING;

    -- Drop the legacy RBAC schema now that its data lives in IAM tables.
    DROP TABLE IF EXISTS user_roles;
    DROP TABLE IF EXISTS role_permissions;
    DROP TABLE IF EXISTS roles;
END $$;
