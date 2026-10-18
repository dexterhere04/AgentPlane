package seed

import (
	"context"
	"errors"
	"fmt"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/iam"
	"github.com/dexterhere04/AgentPlane/internal/provisioning"
	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"github.com/dexterhere04/AgentPlane/internal/users"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultUpstreamSecret is the secret-store key every seeded provider points at.
// A mock-backed demo shares one upstream credential; point --provider-base-url at
// real endpoints and give providers distinct refs for a production-like setup.
const defaultUpstreamSecret = "OPENAI_API_KEY"

// seedPostgres writes the organization into Postgres and returns the IDs of the
// providers it ensured, so the caller can trigger a gateway reload.
func seedPostgres(ctx context.Context, pool *pgxpool.Pool, opts Options, rep *Report) ([]uuid.UUID, error) {
	iamStore := iam.NewStore(pool)
	userStore := users.NewStore(pool)
	apiStore := api.NewStore(pool)
	provStore := proxy.NewProviderStore(pool)
	ruleStore := proxy.NewRoutingRuleStore(pool)
	provisioner := provisioning.NewProvisioner(userStore, apiStore, opts.Pepper)

	if opts.Reset {
		if err := resetDemoData(ctx, pool, iamStore, userStore, provStore, ruleStore); err != nil {
			return nil, err
		}
	}

	// Built-in policies/roles must exist before the org attaches to them.
	if err := iamStore.SeedBuiltins(ctx); err != nil {
		return nil, fmt.Errorf("seed: builtins: %w", err)
	}

	// --- Policies -----------------------------------------------------------
	for _, p := range customPolicies() {
		doc := &iam.PolicyDocument{Version: iam.DefaultVersion, Statement: p.Statements}
		if _, err := iamStore.CreatePolicy(ctx, p.Name, p.Description, doc); err != nil {
			if !errors.Is(err, iam.ErrDuplicatePolicy) {
				return nil, fmt.Errorf("seed: policy %q: %w", p.Name, err)
			}
		}
		rep.Policies++
	}

	// --- Roles --------------------------------------------------------------
	for _, r := range demoRoles() {
		if _, err := iamStore.CreateRole(ctx, r.Name, r.Description, nil, nil); err != nil {
			if !errors.Is(err, iam.ErrDuplicateRole) {
				return nil, fmt.Errorf("seed: role %q: %w", r.Name, err)
			}
		}
		rep.Roles++
	}

	roleList, err := iamStore.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: list roles: %w", err)
	}
	roleID := make(map[string]uuid.UUID, len(roleList))
	for _, r := range roleList {
		roleID[r.Name] = r.ID
	}

	for _, r := range demoRoles() {
		id, ok := roleID[r.Name]
		if !ok {
			return nil, fmt.Errorf("seed: role %q missing after create", r.Name)
		}
		for _, pol := range r.Policies {
			if err := iamStore.AttachPolicy(ctx, pol, iam.PrincipalRole, id); err != nil {
				return nil, fmt.Errorf("seed: attach %q to role %q: %w", pol, r.Name, err)
			}
		}
	}

	// An explicit deny on the built-in member role demonstrates deny-wins.
	if id, ok := roleID["member"]; ok {
		if err := iamStore.AttachPolicy(ctx, "APDenyExperimental", iam.PrincipalRole, id); err != nil {
			return nil, fmt.Errorf("seed: attach deny to member role: %w", err)
		}
	}

	// --- Groups -------------------------------------------------------------
	for _, g := range demoGroups() {
		if _, err := iamStore.CreateGroup(ctx, g.Name, g.Description); err != nil {
			if !errors.Is(err, iam.ErrDuplicateGroup) {
				return nil, fmt.Errorf("seed: group %q: %w", g.Name, err)
			}
		}
		rep.Groups++
	}

	groupList, err := iamStore.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: list groups: %w", err)
	}
	groupID := make(map[string]uuid.UUID, len(groupList))
	for _, g := range groupList {
		groupID[g.Name] = g.ID
	}

	for _, g := range demoGroups() {
		id, ok := groupID[g.Name]
		if !ok {
			return nil, fmt.Errorf("seed: group %q missing after create", g.Name)
		}
		if err := iamStore.GrantRole(ctx, g.Role, iam.PrincipalGroup, id); err != nil {
			return nil, fmt.Errorf("seed: grant role %q to group %q: %w", g.Role, g.Name, err)
		}
		for _, pol := range g.Policies {
			if err := iamStore.AttachPolicy(ctx, pol, iam.PrincipalGroup, id); err != nil {
				return nil, fmt.Errorf("seed: attach %q to group %q: %w", pol, g.Name, err)
			}
		}
	}

	// --- Users --------------------------------------------------------------
	existingUsers, err := userStore.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: list users: %w", err)
	}
	userID := make(map[string]uuid.UUID, len(demoUsers()))
	for _, u := range existingUsers {
		userID[u.Username] = u.ID
	}

	for _, u := range demoUsers() {
		if id, ok := userID[u.Username]; ok && id != uuid.Nil {
			// Already present (re-run without --reset): keep the identity, no new key.
			opts.logf("  user %s already exists — reusing", u.Username)
			continue
		}

		res, err := provisioner.ProvisionUserWithAPIKey(ctx, users.CreateUserParams{
			Username: u.Username,
			Email:    ptr(u.Email),
		}, "primary key")
		if err != nil {
			if errors.Is(err, users.ErrDuplicateUsername) {
				continue
			}
			return nil, fmt.Errorf("seed: provision %q: %w", u.Username, err)
		}

		userID[u.Username] = res.User.ID
		rep.Users++
		rep.Credentials = append(rep.Credentials, Credential{
			Username:       u.Username,
			Email:          u.Email,
			Team:           u.Team,
			Roles:          append([]string{groupRoleFor(u.Team)}, u.DirectRoles...),
			BoundaryPolicy: u.BoundaryPolicy,
			APIKey:         res.FullKey,
		})
	}

	// Memberships, direct roles/policies and boundaries.
	for _, u := range demoUsers() {
		id := userID[u.Username]
		if id == uuid.Nil {
			continue
		}
		if u.Team != "" {
			if err := iamStore.AddUserToGroup(ctx, u.Team, id); err != nil {
				return nil, fmt.Errorf("seed: add %s to group %s: %w", u.Username, u.Team, err)
			}
		}
		for _, r := range u.DirectRoles {
			if err := iamStore.GrantRole(ctx, r, iam.PrincipalUser, id); err != nil {
				return nil, fmt.Errorf("seed: grant role %q to %s: %w", r, u.Username, err)
			}
		}
		for _, pol := range u.Policies {
			if err := iamStore.AttachPolicy(ctx, pol, iam.PrincipalUser, id); err != nil {
				return nil, fmt.Errorf("seed: attach %q to %s: %w", pol, u.Username, err)
			}
		}
		if u.BoundaryPolicy != "" {
			if err := iamStore.SetUserPermissionsBoundary(ctx, id, u.BoundaryPolicy); err != nil {
				return nil, fmt.Errorf("seed: boundary for %s: %w", u.Username, err)
			}
		}
	}

	// --- Providers ----------------------------------------------------------
	existingProviders, err := provStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: list providers: %w", err)
	}
	providerIDByName := make(map[string]uuid.UUID, len(existingProviders))
	for _, p := range existingProviders {
		providerIDByName[p.Name] = p.ID
	}

	var providerIDs []uuid.UUID
	for _, p := range demoProviders() {
		if id, ok := providerIDByName[p.Name]; ok {
			providerIDs = append(providerIDs, id)
			continue
		}
		rec, err := provStore.Create(ctx, proxy.ProviderRecord{
			Name:                    p.Name,
			BaseURL:                 opts.ProviderBaseURL,
			SecretRef:               defaultUpstreamSecret,
			ProviderGroup:           p.Group,
			Weight:                  p.Weight,
			SupportedModels:         p.Models,
			TimeoutMS:               p.TimeoutMS,
			MaxRetries:              p.MaxRetries,
			BreakerFailureThreshold: p.BreakerFailureThreshold,
			Enabled:                 p.Enabled,
		})
		if err != nil {
			if errors.Is(err, proxy.ErrDuplicateProvider) {
				continue
			}
			return nil, fmt.Errorf("seed: provider %q: %w", p.Name, err)
		}
		providerIDs = append(providerIDs, rec.ID)
		rep.Providers++
	}

	// --- Routing rules ------------------------------------------------------
	existingRules, err := ruleStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: list routing rules: %w", err)
	}
	ruleByName := make(map[string]struct{}, len(existingRules))
	for _, r := range existingRules {
		ruleByName[r.Name] = struct{}{}
	}

	for _, r := range demoRules() {
		if _, ok := ruleByName[r.Name]; ok {
			continue
		}
		_, err := ruleStore.Create(ctx, proxy.RoutingRule{
			Name:                  r.Name,
			Priority:              r.Priority,
			ModelMatcher:          r.ModelMatcher,
			RoleMatcher:           r.RoleMatcher,
			UserMatcher:           r.UserMatcher,
			Action:                r.Action,
			ProviderGroup:         r.ProviderGroup,
			TimeoutRetryOverrides: r.OverrideJSON,
			Enabled:               r.Enabled,
		})
		if err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return nil, fmt.Errorf("seed: routing rule %q: %w", r.Name, err)
		}
		rep.RoutingRules++
	}

	if err := backdateDemoRecords(ctx, pool); err != nil {
		return nil, err
	}

	return providerIDs, nil
}

// resetDemoData removes every entity this package creates so a --reset run
// starts from a clean slate. Dangling polymorphic IAM rows for the demo groups
// are removed explicitly because principal_id carries no foreign key.
func resetDemoData(
	ctx context.Context,
	pool *pgxpool.Pool,
	iamStore *iam.Store,
	userStore *users.Store,
	provStore *proxy.ProviderStore,
	ruleStore *proxy.RoutingRuleStore,
) error {
	// Users first: this cascades to their keys, attachments, grants, memberships.
	userList, err := userStore.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("seed: reset list users: %w", err)
	}
	demoUsernames := demoUsernameSet()
	for _, u := range userList {
		if !demoUsernames[u.Username] {
			continue
		}
		if err := userStore.DeleteUser(ctx, u.ID); err != nil && !errors.Is(err, users.ErrUserNotFound) {
			return fmt.Errorf("seed: reset delete user %q: %w", u.Username, err)
		}
	}

	// Groups: clear polymorphic rows, then the group itself.
	groupNames := demoGroupNames()
	if _, err := pool.Exec(ctx,
		`DELETE FROM iam_role_assignments WHERE principal_type = 'group'
		   AND principal_id IN (SELECT id FROM iam_groups WHERE name = ANY($1))`, groupNames); err != nil {
		return fmt.Errorf("seed: reset group role grants: %w", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM iam_policy_attachments WHERE principal_type = 'group'
		   AND principal_id IN (SELECT id FROM iam_groups WHERE name = ANY($1))`, groupNames); err != nil {
		return fmt.Errorf("seed: reset group attachments: %w", err)
	}
	for _, name := range groupNames {
		if err := iamStore.DeleteGroup(ctx, name); err != nil && !errors.Is(err, iam.ErrGroupNotFound) {
			return fmt.Errorf("seed: reset group %q: %w", name, err)
		}
	}

	for _, name := range demoRoleNames() {
		if err := iamStore.DeleteRole(ctx, name); err != nil && !errors.Is(err, iam.ErrRoleNotFound) {
			return fmt.Errorf("seed: reset role %q: %w", name, err)
		}
	}
	for _, name := range demoPolicyNames() {
		if err := iamStore.DeletePolicy(ctx, name); err != nil && !errors.Is(err, iam.ErrPolicyNotFound) {
			return fmt.Errorf("seed: reset policy %q: %w", name, err)
		}
	}

	provList, err := provStore.List(ctx)
	if err != nil {
		return fmt.Errorf("seed: reset list providers: %w", err)
	}
	providerNames := demoProviderNames()
	for _, p := range provList {
		if !providerNames[p.Name] {
			continue
		}
		if err := provStore.Delete(ctx, p.ID); err != nil && !errors.Is(err, proxy.ErrProviderNotFound) {
			return fmt.Errorf("seed: reset provider %q: %w", p.Name, err)
		}
	}

	ruleList, err := ruleStore.List(ctx)
	if err != nil {
		return fmt.Errorf("seed: reset list routing rules: %w", err)
	}
	ruleNames := demoRuleNames()
	for _, r := range ruleList {
		if !ruleNames[r.Name] {
			continue
		}
		if err := ruleStore.Delete(ctx, r.ID); err != nil {
			return fmt.Errorf("seed: reset routing rule %q: %w", r.Name, err)
		}
	}

	return nil
}

// backdateDemoRecords spreads demo identities over the last ~10 months so the
// admin lists look like an organization that has been running, not one created
// in a single second.
func backdateDemoRecords(ctx context.Context, pool *pgxpool.Pool) error {
	usernames := demoUsernameList()
	if _, err := pool.Exec(ctx,
		`UPDATE users
		    SET created_at = now() - (random() * interval '300 days'),
		        updated_at = now()
		  WHERE username = ANY($1)`, usernames); err != nil {
		return fmt.Errorf("seed: backdate users: %w", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE api_keys k
		    SET created_at = u.created_at,
		        last_used_at = now() - (random() * interval '2 days')
		   FROM users u
		  WHERE k.user_id = u.id AND u.username = ANY($1)`, usernames); err != nil {
		return fmt.Errorf("seed: backdate api keys: %w", err)
	}
	return nil
}

// --- helpers ---------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func groupRoleFor(team string) string {
	for _, g := range demoGroups() {
		if g.Name == team {
			return g.Role
		}
	}
	return ""
}

func demoUsernameSet() map[string]bool {
	set := make(map[string]bool)
	for _, u := range demoUsers() {
		set[u.Username] = true
	}
	return set
}

func demoUsernameList() []string {
	out := make([]string, 0, len(demoUsers()))
	for _, u := range demoUsers() {
		out = append(out, u.Username)
	}
	return out
}

func demoGroupNames() []string {
	out := make([]string, 0, len(demoGroups()))
	for _, g := range demoGroups() {
		out = append(out, g.Name)
	}
	return out
}

func demoRoleNames() []string {
	out := make([]string, 0, len(demoRoles()))
	for _, r := range demoRoles() {
		out = append(out, r.Name)
	}
	return out
}

func demoPolicyNames() []string {
	out := make([]string, 0, len(customPolicies()))
	for _, p := range customPolicies() {
		out = append(out, p.Name)
	}
	return out
}

func demoProviderNames() map[string]bool {
	set := make(map[string]bool)
	for _, p := range demoProviders() {
		set[p.Name] = true
	}
	return set
}

func demoRuleNames() map[string]bool {
	set := make(map[string]bool)
	for _, r := range demoRules() {
		set[r.Name] = true
	}
	return set
}
