package handlers

import (
	"log"
	"net/http"

	"github.com/dexterhere04/AgentPlane/internal/policy/rbac"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
)

// adminUserItem is the JSON shape for a user in the admin user-management
// view. It contains only public metadata plus the user's assigned roles.
type adminUserItem struct {
	ID        string   `json:"id"`
	Username  string   `json:"username"`
	Email     string   `json:"email,omitempty"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
	Roles     []string `json:"roles"`
}

// ListUsers returns every user with the roles assigned to them. It is
// admin-gated by the caller (cmd/server/main.go wraps it in
// auth.AdminMiddleware).
//
// The users table and the RBAC tables are owned by separate stores, so this
// handler composes users.Store.ListUsers with a single batched
// rbac.Store.RolesByUserIDs lookup rather than joining across package
// boundaries in a store.
func ListUsers(userStore *users.Store, rbacStore *rbac.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userList, err := userStore.ListUsers(r.Context())
		if err != nil {
			log.Printf("failed to list users: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		ids := make([]uuid.UUID, 0, len(userList))
		for _, u := range userList {
			ids = append(ids, u.ID)
		}

		rolesByUser, err := rbacStore.RolesByUserIDs(r.Context(), ids)
		if err != nil {
			log.Printf("failed to load user roles: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		items := make([]adminUserItem, 0, len(userList))
		for _, u := range userList {
			item := adminUserItem{
				ID:        u.ID.String(),
				Username:  u.Username,
				Status:    u.Status,
				CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				Roles:     rolesByUser[u.ID],
			}
			if item.Roles == nil {
				item.Roles = []string{}
			}
			if u.Email != nil {
				item.Email = *u.Email
			}
			items = append(items, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"users": items})
	})
}
