package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/policy/rbac"
	"github.com/google/uuid"
)

// This file provides the admin HTTP surface for managing RBAC roles,
// permissions, and user-role assignments. Every handler here is mounted
// behind auth.AdminMiddleware in cmd/server/main.go.

type roleResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type createRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type permissionRequest struct {
	Permission string `json:"permission"`
}

type assignRoleRequest struct {
	Role string `json:"role"`
}

// ListRoles returns every role with its permissions.
func ListRoles(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, err := store.ListRoles(r.Context())
		if err != nil {
			log.Printf("failed to list roles: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list roles")
			return
		}

		out := make([]roleResponse, 0, len(roles))
		for _, role := range roles {
			permissions := role.Permissions
			if permissions == nil {
				permissions = []string{}
			}
			out = append(out, roleResponse{
				Name:        role.Name,
				Description: role.Description,
				Permissions: permissions,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"roles": out})
	})
}

// CreateRole creates a new role, optionally with an initial permission set.
func CreateRole(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req createRoleRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Name == "" {
			writeJSONError(w, http.StatusBadRequest, "name is required")
			return
		}

		role, err := store.CreateRole(r.Context(), req.Name, req.Description, req.Permissions)
		if err != nil {
			if errors.Is(err, rbac.ErrDuplicateRole) {
				writeJSONError(w, http.StatusConflict, "role already exists")
				return
			}
			log.Printf("failed to create role %q: %v", req.Name, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to create role")
			return
		}

		permissions := role.Permissions
		if permissions == nil {
			permissions = []string{}
		}
		writeJSON(w, http.StatusCreated, roleResponse{
			Name:        role.Name,
			Description: role.Description,
			Permissions: permissions,
		})
	})
}

// AddRolePermission grants a permission to a role.
func AddRolePermission(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roleName := r.PathValue("name")

		var req permissionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Permission == "" {
			writeJSONError(w, http.StatusBadRequest, "permission is required")
			return
		}

		if err := store.AddPermission(r.Context(), roleName, req.Permission); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to add permission to role %q: %v", roleName, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to add permission")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"role":       roleName,
			"permission": req.Permission,
		})
	})
}

// RemoveRolePermission revokes a permission from a role.
func RemoveRolePermission(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roleName := r.PathValue("name")
		permission := r.PathValue("permission")

		if err := store.RemovePermission(r.Context(), roleName, permission); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to remove permission from role %q: %v", roleName, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to remove permission")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"role":       roleName,
			"permission": permission,
		})
	})
}

// AssignUserRole grants a role to a user.
func AssignUserRole(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}

		var req assignRoleRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Role == "" {
			writeJSONError(w, http.StatusBadRequest, "role is required")
			return
		}

		if err := store.AssignRole(r.Context(), userID, req.Role); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to assign role %q to user %s: %v", req.Role, userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to assign role")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID.String(),
			"role":    req.Role,
		})
	})
}

// RevokeUserRole removes a role from a user.
func RevokeUserRole(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}
		roleName := r.PathValue("role")

		if err := store.RevokeRole(r.Context(), userID, roleName); err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to revoke role %q from user %s: %v", roleName, userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to revoke role")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID.String(),
			"role":    roleName,
		})
	})
}

// ListUserRoles returns the role names assigned to a user.
func ListUserRoles(store *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}

		roles, err := store.ListUserRoles(r.Context(), userID)
		if err != nil {
			log.Printf("failed to list roles for user %s: %v", userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list user roles")
			return
		}
		if roles == nil {
			roles = []string{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID.String(),
			"roles":   roles,
		})
	})
}

func parseUserID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
