// Package rbac implements role-based access control on top of the generic
// internal/policy engine. Roles are named sets of permissions; users are
// granted one or more roles. Access is deny-by-default: a user with no
// roles is granted nothing.
package rbac

import (
	"context"
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

// Sentinel errors returned by the store.
var (
	// ErrDuplicateRole is returned when creating a role whose name is taken.
	ErrDuplicateRole = errors.New("rbac: role already exists")
	// ErrRoleNotFound is returned when a referenced role does not exist.
	ErrRoleNotFound = errors.New("rbac: role not found")
	// ErrInvalidRoleName is returned when a role name is empty.
	ErrInvalidRoleName = errors.New("rbac: role name is required")
)

// Role is a named set of permissions.
type Role struct {
	ID          uuid.UUID
	Name        string
	Description string
	Permissions []string
	CreatedAt   time.Time
}

// Store persists RBAC roles, their permissions, and user-role assignments.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// LoadPermissions returns the distinct set of permissions granted to a user
// through all of their assigned roles.
func (s *Store) LoadPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("rbac: user ID is required")
	}

	const query = `
		SELECT DISTINCT rp.permission
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
	`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: failed to load permissions: %w", err)
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("rbac: failed to scan permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: failed to iterate permissions: %w", err)
	}

	return permissions, nil
}

// ListRoles returns every role with its permissions, ordered by name.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	const query = `
		SELECT r.id, r.name, COALESCE(r.description, ''), r.created_at, rp.permission
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		ORDER BY r.name, rp.permission
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rbac: failed to list roles: %w", err)
	}
	defer rows.Close()

	var (
		order []string
		byID  = make(map[uuid.UUID]*Role)
	)

	for rows.Next() {
		var (
			id          uuid.UUID
			name        string
			description string
			createdAt   time.Time
			permission  *string
		)
		if err := rows.Scan(&id, &name, &description, &createdAt, &permission); err != nil {
			return nil, fmt.Errorf("rbac: failed to scan role: %w", err)
		}

		role, ok := byID[id]
		if !ok {
			role = &Role{ID: id, Name: name, Description: description, CreatedAt: createdAt}
			byID[id] = role
			order = append(order, id.String())
		}
		if permission != nil {
			role.Permissions = append(role.Permissions, *permission)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: failed to iterate roles: %w", err)
	}

	roles := make([]Role, 0, len(order))
	for _, id := range order {
		u, _ := uuid.Parse(id)
		roles = append(roles, *byID[u])
	}
	return roles, nil
}

// CreateRole inserts a new role. Permissions may be empty; they can be added
// later with AddPermission.
func (s *Store) CreateRole(ctx context.Context, name, description string, permissions []string) (*Role, error) {
	if name == "" {
		return nil, ErrInvalidRoleName
	}

	const query = `
		INSERT INTO roles (id, name, description, created_at)
		VALUES (gen_random_uuid(), $1, $2, now())
		RETURNING id, name, COALESCE(description, ''), created_at
	`

	var role Role
	err := s.pool.QueryRow(ctx, query, name, description).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolationCode {
			return nil, ErrDuplicateRole
		}
		return nil, fmt.Errorf("rbac: failed to create role: %w", err)
	}

	for _, p := range permissions {
		if p == "" {
			continue
		}
		if err := s.AddPermission(ctx, role.Name, p); err != nil {
			return nil, err
		}
		role.Permissions = append(role.Permissions, p)
	}

	return &role, nil
}

// AddPermission grants a permission to a role. Adding an already-granted
// permission is a no-op.
func (s *Store) AddPermission(ctx context.Context, roleName, permission string) error {
	if permission == "" {
		return fmt.Errorf("rbac: permission is required")
	}

	roleID, err := s.roleIDByName(ctx, roleName)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO role_permissions (role_id, permission)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	if _, err := s.pool.Exec(ctx, query, roleID, permission); err != nil {
		return fmt.Errorf("rbac: failed to add permission: %w", err)
	}
	return nil
}

// RemovePermission revokes a permission from a role.
func (s *Store) RemovePermission(ctx context.Context, roleName, permission string) error {
	roleID, err := s.roleIDByName(ctx, roleName)
	if err != nil {
		return err
	}

	const query = `
		DELETE FROM role_permissions
		WHERE role_id = $1 AND permission = $2
	`
	if _, err := s.pool.Exec(ctx, query, roleID, permission); err != nil {
		return fmt.Errorf("rbac: failed to remove permission: %w", err)
	}
	return nil
}

// AssignRole grants a role to a user. Assigning an already-held role is a
// no-op.
func (s *Store) AssignRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	if userID == uuid.Nil {
		return fmt.Errorf("rbac: user ID is required")
	}

	roleID, err := s.roleIDByName(ctx, roleName)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	if _, err := s.pool.Exec(ctx, query, userID, roleID); err != nil {
		return fmt.Errorf("rbac: failed to assign role: %w", err)
	}
	return nil
}

// RevokeRole removes a role from a user.
func (s *Store) RevokeRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	if userID == uuid.Nil {
		return fmt.Errorf("rbac: user ID is required")
	}

	roleID, err := s.roleIDByName(ctx, roleName)
	if err != nil {
		return err
	}

	const query = `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role_id = $2
	`
	if _, err := s.pool.Exec(ctx, query, userID, roleID); err != nil {
		return fmt.Errorf("rbac: failed to revoke role: %w", err)
	}
	return nil
}

// ListUserRoles returns the role names assigned to a user.
func (s *Store) ListUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("rbac: user ID is required")
	}

	const query = `
		SELECT r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.name
	`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: failed to list user roles: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("rbac: failed to scan user role: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: failed to iterate user roles: %w", err)
	}
	return names, nil
}

// RolesByUserIDs returns the role names assigned to each of the given users,
// keyed by user ID. It performs a single query (avoiding N+1 lookups) and is
// used to annotate a user list with its roles for the admin UI. Users with no
// roles are absent from the map.
func (s *Store) RolesByUserIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string)
	if len(ids) == 0 {
		return out, nil
	}

	const query = `
		SELECT ur.user_id, r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = ANY($1::uuid[])
		ORDER BY ur.user_id, r.name
	`

	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.String()
	}

	rows, err := s.pool.Query(ctx, query, idStrings)
	if err != nil {
		return nil, fmt.Errorf("rbac: failed to load roles by user: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID uuid.UUID
			name   string
		)
		if err := rows.Scan(&userID, &name); err != nil {
			return nil, fmt.Errorf("rbac: failed to scan user role: %w", err)
		}
		out[userID] = append(out[userID], name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: failed to iterate roles by user: %w", err)
	}

	return out, nil
}

// roleIDByName resolves a role name to its ID, returning ErrRoleNotFound if
// it does not exist.
func (s *Store) roleIDByName(ctx context.Context, name string) (uuid.UUID, error) {
	if name == "" {
		return uuid.Nil, ErrInvalidRoleName
	}

	const query = `SELECT id FROM roles WHERE name = $1`

	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, query, name).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrRoleNotFound
		}
		return uuid.Nil, fmt.Errorf("rbac: failed to look up role: %w", err)
	}
	return id, nil
}
