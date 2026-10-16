DROP TABLE IF EXISTS iam_role_assignments;
DROP TABLE IF EXISTS iam_group_memberships;
DROP TABLE IF EXISTS iam_policy_attachments;
DROP TABLE IF EXISTS iam_roles;
DROP TABLE IF EXISTS iam_groups;
DROP TABLE IF EXISTS iam_policy_versions;
DROP TABLE IF EXISTS iam_policies;

ALTER TABLE users DROP COLUMN IF EXISTS permissions_boundary_policy_id;
