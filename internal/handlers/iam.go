package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/iam"
	"github.com/google/uuid"
)

// This file is the admin HTTP surface for IAM: policies, groups, roles,
// attachments, grants, and permission boundaries. Every handler is mounted
// behind the admin authorization wrapper in cmd/server/main.go.

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

type policyView struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     int             `json:"version"`
	IsBuiltin   bool            `json:"is_builtin"`
	Document    json.RawMessage `json:"document"`
}

func toPolicyView(p iam.Policy) policyView {
	raw, err := p.Document.Bytes()
	if err != nil {
		raw = []byte("{}")
	}
	return policyView{
		Name:        p.Name,
		Description: p.Description,
		Version:     p.Version,
		IsBuiltin:   p.IsBuiltin,
		Document:    json.RawMessage(raw),
	}
}

type policyWriteRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Document    json.RawMessage `json:"document"`
}

// ListPolicies returns every policy.
func ListPolicies(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policies, err := store.ListPolicies(r.Context())
		if err != nil {
			log.Printf("failed to list policies: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list policies")
			return
		}
		out := make([]policyView, 0, len(policies))
		for _, p := range policies {
			out = append(out, toPolicyView(p))
		}
		writeJSON(w, http.StatusOK, map[string]any{"policies": out})
	})
}

// CreatePolicy validates and stores a new policy document.
func CreatePolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req policyWriteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Name == "" {
			writeJSONError(w, http.StatusBadRequest, "name is required")
			return
		}
		doc, err := iam.ParseDocument(req.Document)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := store.CreatePolicy(r.Context(), req.Name, req.Description, doc)
		if err != nil {
			switch {
			case errors.Is(err, iam.ErrDuplicatePolicy):
				writeJSONError(w, http.StatusConflict, "policy already exists")
			case errors.Is(err, iam.ErrInvalidPolicy):
				writeJSONError(w, http.StatusBadRequest, err.Error())
			default:
				log.Printf("failed to create policy %q: %v", req.Name, err)
				writeJSONError(w, http.StatusInternalServerError, "failed to create policy")
			}
			return
		}
		writeJSON(w, http.StatusCreated, toPolicyView(*p))
	})
}

// GetPolicy returns a single policy by name.
func GetPolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := store.GetPolicy(r.Context(), r.PathValue("name"))
		if err != nil {
			if errors.Is(err, iam.ErrPolicyNotFound) {
				writeJSONError(w, http.StatusNotFound, "policy not found")
				return
			}
			log.Printf("failed to get policy: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to get policy")
			return
		}
		writeJSON(w, http.StatusOK, toPolicyView(*p))
	})
}

// UpdatePolicy replaces a policy's document with a new validated version.
func UpdatePolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req policyWriteRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		doc, err := iam.ParseDocument(req.Document)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := store.UpdatePolicy(r.Context(), r.PathValue("name"), doc)
		if err != nil {
			switch {
			case errors.Is(err, iam.ErrPolicyNotFound):
				writeJSONError(w, http.StatusNotFound, "policy not found")
			case errors.Is(err, iam.ErrInvalidPolicy):
				writeJSONError(w, http.StatusBadRequest, err.Error())
			default:
				log.Printf("failed to update policy: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to update policy")
			}
			return
		}
		writeJSON(w, http.StatusOK, toPolicyView(*p))
	})
}

// DeletePolicy removes a policy.
func DeletePolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeletePolicy(r.Context(), r.PathValue("name")); err != nil {
			if errors.Is(err, iam.ErrPolicyNotFound) {
				writeJSONError(w, http.StatusNotFound, "policy not found")
				return
			}
			log.Printf("failed to delete policy: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to delete policy")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("name")})
	})
}

// ---------------------------------------------------------------------------
// Policy attachments
// ---------------------------------------------------------------------------

type principalRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

func parsePrincipal(w http.ResponseWriter, req principalRequest) (string, uuid.UUID, bool) {
	if req.PrincipalType != iam.PrincipalUser && req.PrincipalType != iam.PrincipalGroup && req.PrincipalType != iam.PrincipalRole {
		writeJSONError(w, http.StatusBadRequest, "principal_type must be user, group, or role")
		return "", uuid.Nil, false
	}
	id, err := uuid.Parse(req.PrincipalID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid principal_id")
		return "", uuid.Nil, false
	}
	return req.PrincipalType, id, true
}

// AttachPolicy attaches a policy to a principal.
func AttachPolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req principalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ptype, pid, ok := parsePrincipal(w, req)
		if !ok {
			return
		}
		name := r.PathValue("name")
		if err := store.AttachPolicy(r.Context(), name, ptype, pid); err != nil {
			if errors.Is(err, iam.ErrPolicyNotFound) {
				writeJSONError(w, http.StatusNotFound, "policy not found")
				return
			}
			log.Printf("failed to attach policy %q: %v", name, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to attach policy")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policy": name, "principal_type": ptype, "principal_id": pid.String()})
	})
}

// DetachPolicy removes a policy attachment.
func DetachPolicy(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req principalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ptype, pid, ok := parsePrincipal(w, req)
		if !ok {
			return
		}
		name := r.PathValue("name")
		if err := store.DetachPolicy(r.Context(), name, ptype, pid); err != nil {
			log.Printf("failed to detach policy %q: %v", name, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to detach policy")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policy": name, "principal_type": ptype, "principal_id": pid.String()})
	})
}

type attachmentView struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

// PolicyAttachments returns the principals a policy is attached to.
func PolicyAttachments(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		attachments, err := store.AttachedPrincipals(r.Context(), name)
		if err != nil {
			log.Printf("failed to list attachments: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list attachments")
			return
		}
		out := make([]attachmentView, 0, len(attachments))
		for _, a := range attachments {
			out = append(out, attachmentView{PrincipalType: a.PrincipalType, PrincipalID: a.PrincipalID.String()})
		}
		writeJSON(w, http.StatusOK, map[string]any{"policy": name, "attachments": out})
	})
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

type groupView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type nameDescriptionRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListGroups returns every group.
func ListGroups(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groups, err := store.ListGroups(r.Context())
		if err != nil {
			log.Printf("failed to list groups: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list groups")
			return
		}
		out := make([]groupView, 0, len(groups))
		for _, g := range groups {
			out = append(out, groupView{ID: g.ID.String(), Name: g.Name, Description: g.Description})
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": out})
	})
}

// CreateGroup creates a group.
func CreateGroup(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req nameDescriptionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Name == "" {
			writeJSONError(w, http.StatusBadRequest, "name is required")
			return
		}
		g, err := store.CreateGroup(r.Context(), req.Name, req.Description)
		if err != nil {
			if errors.Is(err, iam.ErrDuplicateGroup) {
				writeJSONError(w, http.StatusConflict, "group already exists")
				return
			}
			log.Printf("failed to create group: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to create group")
			return
		}
		writeJSON(w, http.StatusCreated, groupView{ID: g.ID.String(), Name: g.Name, Description: g.Description})
	})
}

// DeleteGroup removes a group.
func DeleteGroup(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteGroup(r.Context(), r.PathValue("name")); err != nil {
			if errors.Is(err, iam.ErrGroupNotFound) {
				writeJSONError(w, http.StatusNotFound, "group not found")
				return
			}
			log.Printf("failed to delete group: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to delete group")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("name")})
	})
}

type groupMemberRequest struct {
	UserID string `json:"user_id"`
}

// AddGroupMember adds a user to a group.
func AddGroupMember(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req groupMemberRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		name := r.PathValue("name")
		if err := store.AddUserToGroup(r.Context(), name, userID); err != nil {
			if errors.Is(err, iam.ErrGroupNotFound) {
				writeJSONError(w, http.StatusNotFound, "group not found")
				return
			}
			log.Printf("failed to add user to group: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to add user to group")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": name, "user_id": userID.String()})
	})
}

// RemoveGroupMember removes a user from a group.
func RemoveGroupMember(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}
		name := r.PathValue("name")
		if err := store.RemoveUserFromGroup(r.Context(), name, userID); err != nil {
			log.Printf("failed to remove user from group: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to remove user from group")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": name, "user_id": userID.String()})
	})
}

// ListGroupMembers returns the members of a group.
func ListGroupMembers(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids, err := store.GroupMembers(r.Context(), r.PathValue("name"))
		if err != nil {
			log.Printf("failed to list group members: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list group members")
			return
		}
		strs := make([]string, 0, len(ids))
		for _, id := range ids {
			strs = append(strs, id.String())
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": r.PathValue("name"), "members": strs})
	})
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

type roleView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Policies    []string `json:"policies"`
	TrustPolicy any      `json:"trust_policy,omitempty"`
}

// ListRoles returns every IAM role with the policies attached to it.
func ListRoles(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roles, err := store.ListRoles(r.Context())
		if err != nil {
			log.Printf("failed to list roles: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list roles")
			return
		}
		out := make([]roleView, 0, len(roles))
		for _, role := range roles {
			policies, err := store.PrincipalPolicies(r.Context(), iam.PrincipalRole, role.ID)
			if err != nil {
				log.Printf("failed to list role policies: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to list roles")
				return
			}
			if policies == nil {
				policies = []string{}
			}
			out = append(out, roleView{ID: role.ID.String(), Name: role.Name, Description: role.Description, Policies: policies})
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": out})
	})
}

type createRoleRequest struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	TrustPolicy         json.RawMessage `json:"trust_policy"`
	PermissionsBoundary string          `json:"permissions_boundary"`
}

// CreateRole creates an IAM role.
func CreateRole(store *iam.Store) http.Handler {
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

		var trust *iam.PolicyDocument
		if len(req.TrustPolicy) > 0 {
			doc, err := iam.ParseDocument(req.TrustPolicy)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid trust_policy")
				return
			}
			trust = doc
		}

		var boundaryID *uuid.UUID
		if req.PermissionsBoundary != "" {
			p, err := store.GetPolicy(r.Context(), req.PermissionsBoundary)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "unknown permissions_boundary policy")
				return
			}
			boundaryID = &p.ID
		}

		role, err := store.CreateRole(r.Context(), req.Name, req.Description, trust, boundaryID)
		if err != nil {
			if errors.Is(err, iam.ErrDuplicateRole) {
				writeJSONError(w, http.StatusConflict, "role already exists")
				return
			}
			log.Printf("failed to create role: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to create role")
			return
		}
		writeJSON(w, http.StatusCreated, roleView{ID: role.ID.String(), Name: role.Name, Description: role.Description, Policies: []string{}})
	})
}

// DeleteRole removes an IAM role.
func DeleteRole(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := store.DeleteRole(r.Context(), r.PathValue("name")); err != nil {
			if errors.Is(err, iam.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to delete role: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to delete role")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("name")})
	})
}

// GrantRole grants a role to a user or group.
func GrantRole(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req principalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.PrincipalType != iam.PrincipalUser && req.PrincipalType != iam.PrincipalGroup {
			writeJSONError(w, http.StatusBadRequest, "principal_type must be user or group")
			return
		}
		pid, err := uuid.Parse(req.PrincipalID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid principal_id")
			return
		}
		name := r.PathValue("name")
		if err := store.GrantRole(r.Context(), name, req.PrincipalType, pid); err != nil {
			if errors.Is(err, iam.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to grant role: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to grant role")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"role": name, "principal_type": req.PrincipalType, "principal_id": pid.String()})
	})
}

// RevokeRole removes a role grant.
func RevokeRole(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req principalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		pid, err := uuid.Parse(req.PrincipalID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid principal_id")
			return
		}
		name := r.PathValue("name")
		if err := store.RevokeRole(r.Context(), name, req.PrincipalType, pid); err != nil {
			log.Printf("failed to revoke role: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to revoke role")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"role": name, "principal_type": req.PrincipalType, "principal_id": pid.String()})
	})
}

// RoleGrants returns the principals a role is granted to.
func RoleGrants(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		grants, err := store.RoleGrants(r.Context(), name)
		if err != nil {
			log.Printf("failed to list role grants: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list role grants")
			return
		}
		out := make([]attachmentView, 0, len(grants))
		for _, g := range grants {
			out = append(out, attachmentView{PrincipalType: g.PrincipalType, PrincipalID: g.PrincipalID.String()})
		}
		writeJSON(w, http.StatusOK, map[string]any{"role": name, "grants": out})
	})
}

// ---------------------------------------------------------------------------
// User roles & permission boundary
// ---------------------------------------------------------------------------

// ListUserRoles returns the role names granted to a user.
func ListUserRoles(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}
		roles, err := store.UserRoles(r.Context(), userID)
		if err != nil {
			log.Printf("failed to list user roles: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list user roles")
			return
		}
		if roles == nil {
			roles = []string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID.String(), "roles": roles})
	})
}

type assignRoleRequest struct {
	Role string `json:"role"`
}

// AssignUserRole grants a role to a user.
func AssignUserRole(store *iam.Store) http.Handler {
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
		if err := store.GrantRole(r.Context(), req.Role, iam.PrincipalUser, userID); err != nil {
			if errors.Is(err, iam.ErrRoleNotFound) {
				writeJSONError(w, http.StatusNotFound, "role not found")
				return
			}
			log.Printf("failed to assign role %q to user %s: %v", req.Role, userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to assign role")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID.String(), "role": req.Role})
	})
}

// RevokeUserRole removes a role from a user.
func RevokeUserRole(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}
		roleName := r.PathValue("role")
		if err := store.RevokeRole(r.Context(), roleName, iam.PrincipalUser, userID); err != nil {
			log.Printf("failed to revoke role %q from user %s: %v", roleName, userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to revoke role")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID.String(), "role": roleName})
	})
}

type boundaryRequest struct {
	Policy string `json:"policy"`
}

// SetUserPermissionsBoundary sets or clears a user's permission boundary. An
// empty "policy" clears it.
func SetUserPermissionsBoundary(store *iam.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r.PathValue("id"))
		if !ok {
			return
		}
		var req boundaryRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := store.SetUserPermissionsBoundary(r.Context(), userID, req.Policy); err != nil {
			log.Printf("failed to set permissions boundary: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to set permissions boundary")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": userID.String(), "policy": req.Policy})
	})
}

// ---------------------------------------------------------------------------
// Shared JSON helpers
// ---------------------------------------------------------------------------

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
