package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresUniqueViolationCode is the Postgres error code for a unique
// constraint violation (23505).
const postgresUniqueViolationCode = "23505"

// principalType values for iam_policy_attachments / iam_role_assignments.
const (
	PrincipalUser  = "user"
	PrincipalGroup = "group"
	PrincipalRole  = "role"
)

// maxPolicyVersions mirrors AWS's five-version cap.
const maxPolicyVersions = 5

// Sentinel errors returned by the store.
var (
	ErrDuplicatePolicy = errors.New("iam: policy already exists")
	ErrPolicyNotFound  = errors.New("iam: policy not found")
	ErrDuplicateRole   = errors.New("iam: role already exists")
	ErrRoleNotFound    = errors.New("iam: role not found")
	ErrDuplicateGroup  = errors.New("iam: group already exists")
	ErrGroupNotFound   = errors.New("iam: group not found")
	ErrInvalidPolicy   = errors.New("iam: invalid policy document")
	ErrInvalidName     = errors.New("iam: name is required")
)

// Policy is a named, reusable policy document.
type Policy struct {
	ID          uuid.UUID
	Name        string
	Description string
	Document    *PolicyDocument
	Version     int
	IsBuiltin   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Group is a collection of users.
type Group struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatedAt   time.Time
}

// Role is an assumable identity carrying policies and an optional trust policy.
type Role struct {
	ID                          uuid.UUID
	Name                        string
	Description                 string
	AssumeRolePolicy            *PolicyDocument
	PermissionsBoundaryPolicyID *uuid.UUID
	CreatedAt                   time.Time
}

// Store persists IAM policies, roles, groups, attachments, assignments, and
// permission boundaries. It satisfies PolicyLoader so it can back Engine.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ---------------------------------------------------------------------------
// PolicyLoader implementation
// ---------------------------------------------------------------------------

// LoadPrincipalPolicies returns every policy document that applies to a user:
// policies attached directly to the user, to any group the user belongs to,
// and to any role granted to the user (directly or via a group).
func (s *Store) LoadPrincipalPolicies(ctx context.Context, userID uuid.UUID) ([]*PolicyDocument, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("iam: user ID is required")
	}

	const query = `
		WITH user_groups AS (
			SELECT group_id FROM iam_group_memberships WHERE user_id = $1
		),
		user_roles AS (
			SELECT role_id FROM iam_role_assignments
			WHERE (principal_type = 'user' AND principal_id = $1)
			   OR (principal_type = 'group' AND principal_id IN (SELECT group_id FROM user_groups))
		)
		SELECT DISTINCT p.id, p.name, COALESCE(p.description, ''), p.document,
		                p.version, p.is_builtin, p.created_at, p.updated_at
		FROM iam_policies p
		JOIN iam_policy_attachments a ON a.policy_id = p.id
		WHERE (a.principal_type = 'user' AND a.principal_id = $1)
		   OR (a.principal_type = 'group' AND a.principal_id IN (SELECT group_id FROM user_groups))
		   OR (a.principal_type = 'role' AND a.principal_id IN (SELECT role_id FROM user_roles))
	`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to load principal policies: %w", err)
	}
	defer rows.Close()

	var policies []*PolicyDocument
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p.Document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate principal policies: %w", err)
	}
	return policies, nil
}

// LoadPermissionsBoundary returns the user's permission boundary, or
// (nil, nil) when none is set.
func (s *Store) LoadPermissionsBoundary(ctx context.Context, userID uuid.UUID) (*PolicyDocument, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("iam: user ID is required")
	}

	const query = `
		SELECT p.id, p.name, COALESCE(p.description, ''), p.document,
		       p.version, p.is_builtin, p.created_at, p.updated_at
		FROM users u
		JOIN iam_policies p ON p.id = u.permissions_boundary_policy_id
		WHERE u.id = $1
	`

	p, err := scanPolicy(s.pool.QueryRow(ctx, query, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p.Document, nil
}

// ---------------------------------------------------------------------------
// Policy CRUD
// ---------------------------------------------------------------------------

// CreatePolicy validates and stores a new policy (version 1).
func (s *Store) CreatePolicy(ctx context.Context, name, description string, doc *PolicyDocument) (*Policy, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	if err := Validate(doc, ValidateOptions{}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("iam: marshal document: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insert = `
		INSERT INTO iam_policies (name, description, document, version)
		VALUES ($1, $2, $3, 1)
		RETURNING id, name, COALESCE(description, ''), document, version, is_builtin, created_at, updated_at
	`
	p, err := scanPolicy(tx.QueryRow(ctx, insert, name, description, raw))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			return nil, ErrDuplicatePolicy
		}
		return nil, fmt.Errorf("iam: failed to create policy: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_policy_versions (policy_id, version, document, is_default)
		VALUES ($1, 1, $2, TRUE)
	`, p.ID, raw); err != nil {
		return nil, fmt.Errorf("iam: failed to record policy version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("iam: commit: %w", err)
	}
	return p, nil
}

// UpdatePolicy replaces a policy's document with a new validated version,
// evicting the oldest version beyond maxPolicyVersions.
func (s *Store) UpdatePolicy(ctx context.Context, name string, doc *PolicyDocument) (*Policy, error) {
	if err := Validate(doc, ValidateOptions{}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("iam: marshal document: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("iam: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		id      uuid.UUID
		version int
	)
	if err := tx.QueryRow(ctx, `SELECT id, version FROM iam_policies WHERE name = $1 FOR UPDATE`, name).Scan(&id, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("iam: failed to load policy: %w", err)
	}

	newVersion := version + 1
	if _, err := tx.Exec(ctx, `UPDATE iam_policy_versions SET is_default = FALSE WHERE policy_id = $1`, id); err != nil {
		return nil, fmt.Errorf("iam: failed to clear default version: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO iam_policy_versions (policy_id, version, document, is_default)
		VALUES ($1, $2, $3, TRUE)
	`, id, newVersion, raw); err != nil {
		return nil, fmt.Errorf("iam: failed to record policy version: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM iam_policy_versions WHERE policy_id = $1 AND version <= $2`, id, newVersion-maxPolicyVersions); err != nil {
		return nil, fmt.Errorf("iam: failed to evict old policy versions: %w", err)
	}

	const update = `
		UPDATE iam_policies SET document = $2, version = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, name, COALESCE(description, ''), document, version, is_builtin, created_at, updated_at
	`
	p, err := scanPolicy(tx.QueryRow(ctx, update, id, raw, newVersion))
	if err != nil {
		return nil, fmt.Errorf("iam: failed to update policy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("iam: commit: %w", err)
	}
	return p, nil
}

// ListPolicies returns every policy, ordered by name.
func (s *Store) ListPolicies(ctx context.Context) ([]Policy, error) {
	const query = `
		SELECT id, name, COALESCE(description, ''), document, version, is_builtin, created_at, updated_at
		FROM iam_policies
		ORDER BY name
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list policies: %w", err)
	}
	defer rows.Close()

	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate policies: %w", err)
	}
	return out, nil
}

// GetPolicy returns a single policy by name.
func (s *Store) GetPolicy(ctx context.Context, name string) (*Policy, error) {
	const query = `
		SELECT id, name, COALESCE(description, ''), document, version, is_builtin, created_at, updated_at
		FROM iam_policies WHERE name = $1
	`
	p, err := scanPolicy(s.pool.QueryRow(ctx, query, name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("iam: failed to get policy: %w", err)
	}
	return p, nil
}

// DeletePolicy removes a policy and, by cascade, its versions and attachments.
func (s *Store) DeletePolicy(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM iam_policies WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("iam: failed to delete policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrPolicyNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

// AttachPolicy attaches a policy to a user, group, or role (idempotent).
func (s *Store) AttachPolicy(ctx context.Context, policyName, principalType string, principalID uuid.UUID) error {
	if principalID == uuid.Nil {
		return fmt.Errorf("iam: principal ID is required")
	}
	if err := validPrincipalType(principalType); err != nil {
		return err
	}

	const query = `
		INSERT INTO iam_policy_attachments (policy_id, principal_type, principal_id)
		SELECT id, $2, $3 FROM iam_policies WHERE name = $1
		ON CONFLICT DO NOTHING
	`
	tag, err := s.pool.Exec(ctx, query, policyName, principalType, principalID)
	if err != nil {
		return fmt.Errorf("iam: failed to attach policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the policy does not exist or the attachment already exists.
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam_policies WHERE name = $1)`, policyName).Scan(&exists); err != nil {
			return fmt.Errorf("iam: failed to check policy: %w", err)
		}
		if !exists {
			return ErrPolicyNotFound
		}
	}
	return nil
}

// DetachPolicy removes a policy attachment.
func (s *Store) DetachPolicy(ctx context.Context, policyName, principalType string, principalID uuid.UUID) error {
	if err := validPrincipalType(principalType); err != nil {
		return err
	}
	const query = `
		DELETE FROM iam_policy_attachments a
		USING iam_policies p
		WHERE a.policy_id = p.id AND p.name = $1
		  AND a.principal_type = $2 AND a.principal_id = $3
	`
	if _, err := s.pool.Exec(ctx, query, policyName, principalType, principalID); err != nil {
		return fmt.Errorf("iam: failed to detach policy: %w", err)
	}
	return nil
}

// PrincipalPolicies returns the names of policies attached directly to a
// principal (not transitively through groups/roles).
func (s *Store) PrincipalPolicies(ctx context.Context, principalType string, principalID uuid.UUID) ([]string, error) {
	if err := validPrincipalType(principalType); err != nil {
		return nil, err
	}
	const query = `
		SELECT p.name
		FROM iam_policy_attachments a
		JOIN iam_policies p ON p.id = a.policy_id
		WHERE a.principal_type = $1 AND a.principal_id = $2
		ORDER BY p.name
	`
	rows, err := s.pool.Query(ctx, query, principalType, principalID)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list principal policies: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("iam: failed to scan policy name: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate principal policies: %w", err)
	}
	return names, nil
}

// AttachedPrincipals returns the principals a policy is attached to.
func (s *Store) AttachedPrincipals(ctx context.Context, policyName string) ([]Attachment, error) {
	const query = `
		SELECT a.principal_type, a.principal_id
		FROM iam_policy_attachments a
		JOIN iam_policies p ON p.id = a.policy_id
		WHERE p.name = $1
		ORDER BY a.principal_type, a.principal_id
	`
	rows, err := s.pool.Query(ctx, query, policyName)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list principals: %w", err)
	}
	defer rows.Close()

	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.PrincipalType, &a.PrincipalID); err != nil {
			return nil, fmt.Errorf("iam: failed to scan principal: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate principals: %w", err)
	}
	return out, nil
}

// Attachment is one policy-to-principal link.
type Attachment struct {
	PrincipalType string
	PrincipalID   uuid.UUID
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

// CreateGroup inserts a new group.
func (s *Store) CreateGroup(ctx context.Context, name, description string) (*Group, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	const query = `
		INSERT INTO iam_groups (name, description)
		VALUES ($1, $2)
		RETURNING id, name, COALESCE(description, ''), created_at
	`
	var g Group
	err := s.pool.QueryRow(ctx, query, name, description).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			return nil, ErrDuplicateGroup
		}
		return nil, fmt.Errorf("iam: failed to create group: %w", err)
	}
	return &g, nil
}

// ListGroups returns every group ordered by name.
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	const query = `SELECT id, name, COALESCE(description, ''), created_at FROM iam_groups ORDER BY name`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("iam: failed to scan group: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate groups: %w", err)
	}
	return out, nil
}

// DeleteGroup removes a group and, by cascade, its memberships and attachments.
func (s *Store) DeleteGroup(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM iam_groups WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("iam: failed to delete group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGroupNotFound
	}
	return nil
}

// AddUserToGroup adds a user to a group by name (idempotent).
func (s *Store) AddUserToGroup(ctx context.Context, groupName string, userID uuid.UUID) error {
	const query = `
		INSERT INTO iam_group_memberships (group_id, user_id)
		SELECT id, $2 FROM iam_groups WHERE name = $1
		ON CONFLICT DO NOTHING
	`
	tag, err := s.pool.Exec(ctx, query, groupName, userID)
	if err != nil {
		return fmt.Errorf("iam: failed to add user to group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam_groups WHERE name = $1)`, groupName).Scan(&exists); err != nil {
			return fmt.Errorf("iam: failed to check group: %w", err)
		}
		if !exists {
			return ErrGroupNotFound
		}
	}
	return nil
}

// RemoveUserFromGroup removes a user from a group.
func (s *Store) RemoveUserFromGroup(ctx context.Context, groupName string, userID uuid.UUID) error {
	const query = `
		DELETE FROM iam_group_memberships m
		USING iam_groups g
		WHERE m.group_id = g.id AND g.name = $1 AND m.user_id = $2
	`
	if _, err := s.pool.Exec(ctx, query, groupName, userID); err != nil {
		return fmt.Errorf("iam: failed to remove user from group: %w", err)
	}
	return nil
}

// GroupMembers returns the user IDs in a group.
func (s *Store) GroupMembers(ctx context.Context, groupName string) ([]uuid.UUID, error) {
	const query = `
		SELECT m.user_id
		FROM iam_group_memberships m
		JOIN iam_groups g ON g.id = m.group_id
		WHERE g.name = $1
		ORDER BY m.user_id
	`
	rows, err := s.pool.Query(ctx, query, groupName)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list group members: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("iam: failed to scan member: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate members: %w", err)
	}
	return ids, nil
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

// CreateRole inserts a new role. trustPolicy and boundaryID are optional.
func (s *Store) CreateRole(ctx context.Context, name, description string, trustPolicy *PolicyDocument, boundaryID *uuid.UUID) (*Role, error) {
	if name == "" {
		return nil, ErrInvalidName
	}
	var trust []byte
	if trustPolicy != nil {
		b, err := json.Marshal(trustPolicy)
		if err != nil {
			return nil, fmt.Errorf("iam: marshal trust policy: %w", err)
		}
		trust = b
	}

	const query = `
		INSERT INTO iam_roles (name, description, assume_role_policy_document, permissions_boundary_policy_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, COALESCE(description, ''), assume_role_policy_document, permissions_boundary_policy_id, created_at
	`
	var (
		r       Role
		trustJS []byte
		bound   *uuid.UUID
	)
	err := s.pool.QueryRow(ctx, query, name, description, trust, boundaryID).Scan(
		&r.ID, &r.Name, &r.Description, &trustJS, &bound, &r.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			return nil, ErrDuplicateRole
		}
		return nil, fmt.Errorf("iam: failed to create role: %w", err)
	}
	if len(trustJS) > 0 {
		doc, err := ParseDocument(trustJS)
		if err != nil {
			return nil, fmt.Errorf("iam: parse stored trust policy: %w", err)
		}
		r.AssumeRolePolicy = doc
	}
	r.PermissionsBoundaryPolicyID = bound
	return &r, nil
}

// ListRoles returns every role ordered by name.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	const query = `
		SELECT id, name, COALESCE(description, ''), assume_role_policy_document, permissions_boundary_policy_id, created_at
		FROM iam_roles ORDER BY name
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list roles: %w", err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var (
			r       Role
			trustJS []byte
			bound   *uuid.UUID
		)
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &trustJS, &bound, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("iam: failed to scan role: %w", err)
		}
		if len(trustJS) > 0 {
			if doc, err := ParseDocument(trustJS); err == nil {
				r.AssumeRolePolicy = doc
			}
		}
		r.PermissionsBoundaryPolicyID = bound
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate roles: %w", err)
	}
	return out, nil
}

// DeleteRole removes a role and, by cascade, its assignments and attachments.
func (s *Store) DeleteRole(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM iam_roles WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("iam: failed to delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// GrantRole grants a role to a user or group (idempotent).
func (s *Store) GrantRole(ctx context.Context, roleName, principalType string, principalID uuid.UUID) error {
	if principalType != PrincipalUser && principalType != PrincipalGroup {
		return fmt.Errorf("iam: role grants only support user or group principals, got %q", principalType)
	}
	const query = `
		INSERT INTO iam_role_assignments (role_id, principal_type, principal_id)
		SELECT id, $2, $3 FROM iam_roles WHERE name = $1
		ON CONFLICT DO NOTHING
	`
	tag, err := s.pool.Exec(ctx, query, roleName, principalType, principalID)
	if err != nil {
		return fmt.Errorf("iam: failed to grant role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM iam_roles WHERE name = $1)`, roleName).Scan(&exists); err != nil {
			return fmt.Errorf("iam: failed to check role: %w", err)
		}
		if !exists {
			return ErrRoleNotFound
		}
	}
	return nil
}

// RevokeRole removes a role grant.
func (s *Store) RevokeRole(ctx context.Context, roleName, principalType string, principalID uuid.UUID) error {
	const query = `
		DELETE FROM iam_role_assignments ra
		USING iam_roles r
		WHERE ra.role_id = r.id AND r.name = $1
		  AND ra.principal_type = $2 AND ra.principal_id = $3
	`
	if _, err := s.pool.Exec(ctx, query, roleName, principalType, principalID); err != nil {
		return fmt.Errorf("iam: failed to revoke role: %w", err)
	}
	return nil
}

// RoleGrants returns the principals a role is granted to.
func (s *Store) RoleGrants(ctx context.Context, roleName string) ([]Attachment, error) {
	const query = `
		SELECT ra.principal_type, ra.principal_id
		FROM iam_role_assignments ra
		JOIN iam_roles r ON r.id = ra.role_id
		WHERE r.name = $1
		ORDER BY ra.principal_type, ra.principal_id
	`
	rows, err := s.pool.Query(ctx, query, roleName)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list role grants: %w", err)
	}
	defer rows.Close()

	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.PrincipalType, &a.PrincipalID); err != nil {
			return nil, fmt.Errorf("iam: failed to scan grant: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate grants: %w", err)
	}
	return out, nil
}

// UserRoles returns the names of roles granted to a user (directly or via a
// group). It is used by the routing layer, which keys behaviour on role names.
func (s *Store) UserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	const query = `
		WITH user_groups AS (
			SELECT group_id FROM iam_group_memberships WHERE user_id = $1
		)
		SELECT DISTINCT r.name
		FROM iam_roles r
		JOIN iam_role_assignments ra ON ra.role_id = r.id
		WHERE (ra.principal_type = 'user' AND ra.principal_id = $1)
		   OR (ra.principal_type = 'group' AND ra.principal_id IN (SELECT group_id FROM user_groups))
		ORDER BY r.name
	`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to list user roles: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("iam: failed to scan role name: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate user roles: %w", err)
	}
	return names, nil
}

// UserRolesByIDs returns the role names granted to each of the given users
// (directly or via a group), keyed by user ID. It performs a single query
// (avoiding N+1 lookups) and is used to annotate a user list with its roles.
// Users with no roles are absent from the map.
func (s *Store) UserRolesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string)
	if len(ids) == 0 {
		return out, nil
	}

	const query = `
		WITH direct AS (
			SELECT ra.principal_id AS user_id, r.name
			FROM iam_role_assignments ra
			JOIN iam_roles r ON r.id = ra.role_id
			WHERE ra.principal_type = 'user' AND ra.principal_id = ANY($1::uuid[])
		),
		via_group AS (
			SELECT m.user_id, r.name
			FROM iam_role_assignments ra
			JOIN iam_roles r ON r.id = ra.role_id
			JOIN iam_group_memberships m ON m.group_id = ra.principal_id
			WHERE ra.principal_type = 'group' AND m.user_id = ANY($1::uuid[])
		)
		SELECT DISTINCT user_id, name FROM (
			SELECT user_id, name FROM direct
			UNION
			SELECT user_id, name FROM via_group
		) u
		ORDER BY user_id, name
	`

	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.String()
	}

	rows, err := s.pool.Query(ctx, query, idStrings)
	if err != nil {
		return nil, fmt.Errorf("iam: failed to load roles by user: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID uuid.UUID
			name   string
		)
		if err := rows.Scan(&userID, &name); err != nil {
			return nil, fmt.Errorf("iam: failed to scan user role: %w", err)
		}
		out[userID] = append(out[userID], name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: failed to iterate roles by user: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Permission boundaries
// ---------------------------------------------------------------------------

// SetUserPermissionsBoundary sets (or clears, when policyName is empty) a
// user's permission boundary.
func (s *Store) SetUserPermissionsBoundary(ctx context.Context, userID uuid.UUID, policyName string) error {
	if userID == uuid.Nil {
		return fmt.Errorf("iam: user ID is required")
	}
	if policyName == "" {
		if _, err := s.pool.Exec(ctx, `UPDATE users SET permissions_boundary_policy_id = NULL WHERE id = $1`, userID); err != nil {
			return fmt.Errorf("iam: failed to clear boundary: %w", err)
		}
		return nil
	}

	const query = `
		UPDATE users SET permissions_boundary_policy_id = (SELECT id FROM iam_policies WHERE name = $2)
		WHERE id = $1
	`
	if _, err := s.pool.Exec(ctx, query, userID, policyName); err != nil {
		return fmt.Errorf("iam: failed to set boundary: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanPolicy reads a policy row (id, name, description, document, version,
// is_builtin, created_at, updated_at) into a Policy.
func scanPolicy(row rowScanner) (*Policy, error) {
	var (
		p       Policy
		docJSON []byte
	)
	if err := row.Scan(
		&p.ID, &p.Name, &p.Description, &docJSON,
		&p.Version, &p.IsBuiltin, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	doc, err := ParseDocument(docJSON)
	if err != nil {
		return nil, fmt.Errorf("iam: parse stored document for policy %q: %w", p.Name, err)
	}
	p.Document = doc
	return &p, nil
}

// validPrincipalType checks principalType against the allowed set.
func validPrincipalType(principalType string) error {
	switch principalType {
	case PrincipalUser, PrincipalGroup, PrincipalRole:
		return nil
	default:
		return fmt.Errorf("iam: invalid principal type %q", principalType)
	}
}
