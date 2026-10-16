package iam

import "strings"

// Actions use the "ap:" service prefix, mirroring AWS's "service:ActionName"
// convention (e.g. s3:GetObject → ap:InvokeModel). Wildcards ('*', '?') are
// legal anywhere in an action pattern.
const actionPrefix = "ap:"

// Canonical gateway actions. These mirror the operations the gateway enforces
// today (see cmd/server/main.go and internal/handlers) plus the IAM
// administration surface.
const (
	// Gateway data-plane.
	ActionInvokeChat  = "ap:InvokeChat"
	ActionInvokeModel = "ap:InvokeModel"

	// Provider administration.
	ActionListProviders   = "ap:ListProviders"
	ActionCreateProvider  = "ap:CreateProvider"
	ActionUpdateProvider  = "ap:UpdateProvider"
	ActionDeleteProvider  = "ap:DeleteProvider"
	ActionEnableProvider  = "ap:EnableProvider"
	ActionDisableProvider = "ap:DisableProvider"

	// Routing-rule administration.
	ActionListRoutingRules   = "ap:ListRoutingRules"
	ActionCreateRoutingRule  = "ap:CreateRoutingRule"
	ActionUpdateRoutingRule  = "ap:UpdateRoutingRule"
	ActionDeleteRoutingRule  = "ap:DeleteRoutingRule"
	ActionEnableRoutingRule  = "ap:EnableRoutingRule"
	ActionDisableRoutingRule = "ap:DisableRoutingRule"

	// API-key and user administration.
	ActionListApiKeys   = "ap:ListApiKeys"
	ActionRevokeApiKey  = "ap:RevokeApiKey"
	ActionProvisionUser = "ap:ProvisionUser"
	ActionListUsers     = "ap:ListUsers"
	ActionDeleteUser    = "ap:DeleteUser"

	// Secrets, metrics, observability.
	ActionManageSecrets = "ap:ManageSecrets"
	ActionViewMetrics   = "ap:ViewMetrics"

	// IAM administration (policies).
	ActionListPolicies = "ap:ListPolicies"
	ActionGetPolicy    = "ap:GetPolicy"
	ActionCreatePolicy = "ap:CreatePolicy"
	ActionUpdatePolicy = "ap:UpdatePolicy"
	ActionDeletePolicy = "ap:DeletePolicy"
	ActionAttachPolicy = "ap:AttachPolicy"
	ActionDetachPolicy = "ap:DetachPolicy"

	// IAM administration (groups).
	ActionListGroups          = "ap:ListGroups"
	ActionGetGroup            = "ap:GetGroup"
	ActionCreateGroup         = "ap:CreateGroup"
	ActionDeleteGroup         = "ap:DeleteGroup"
	ActionAddUserToGroup      = "ap:AddUserToGroup"
	ActionRemoveUserFromGroup = "ap:RemoveUserFromGroup"

	// IAM administration (roles).
	ActionListRoles              = "ap:ListRoles"
	ActionGetRole                = "ap:GetRole"
	ActionCreateRole             = "ap:CreateRole"
	ActionDeleteRole             = "ap:DeleteRole"
	ActionGrantRole              = "ap:GrantRole"
	ActionRevokeRole             = "ap:RevokeRole"
	ActionSetPermissionsBoundary = "ap:SetPermissionsBoundary"

	// Reserved: role assumption (STS-style), deferred to v2.
	ActionAssumeRole = "ap:AssumeRole"
)

// KnownActions is the set of actions the gateway enforces. Unknown actions are
// rejected at policy-write time unless validation opts to allow them.
var KnownActions = map[string]bool{
	ActionInvokeChat:  true,
	ActionInvokeModel: true,

	ActionListProviders:   true,
	ActionCreateProvider:  true,
	ActionUpdateProvider:  true,
	ActionDeleteProvider:  true,
	ActionEnableProvider:  true,
	ActionDisableProvider: true,

	ActionListRoutingRules:   true,
	ActionCreateRoutingRule:  true,
	ActionUpdateRoutingRule:  true,
	ActionDeleteRoutingRule:  true,
	ActionEnableRoutingRule:  true,
	ActionDisableRoutingRule: true,

	ActionListApiKeys:   true,
	ActionRevokeApiKey:  true,
	ActionProvisionUser: true,
	ActionListUsers:     true,
	ActionDeleteUser:    true,

	ActionManageSecrets: true,
	ActionViewMetrics:   true,

	ActionListPolicies: true,
	ActionGetPolicy:    true,
	ActionCreatePolicy: true,
	ActionUpdatePolicy: true,
	ActionDeletePolicy: true,
	ActionAttachPolicy: true,
	ActionDetachPolicy: true,

	ActionListGroups:          true,
	ActionGetGroup:            true,
	ActionCreateGroup:         true,
	ActionDeleteGroup:         true,
	ActionAddUserToGroup:      true,
	ActionRemoveUserFromGroup: true,

	ActionListRoles:              true,
	ActionGetRole:                true,
	ActionCreateRole:             true,
	ActionDeleteRole:             true,
	ActionGrantRole:              true,
	ActionRevokeRole:             true,
	ActionSetPermissionsBoundary: true,

	ActionAssumeRole: true,
}

// MatchAction reports whether the action pattern grants the concrete action.
func MatchAction(pattern, action string) bool {
	if pattern == "" || action == "" {
		return false
	}
	return matchGlob(pattern, action)
}

// ValidActionPattern reports whether s is a syntactically valid action pattern:
// the "*" shorthand, or "ap:{Name}" where Name contains only letters, digits,
// underscores, or wildcards.
func ValidActionPattern(s string) bool {
	if s == "*" {
		return true
	}
	if !strings.HasPrefix(s, actionPrefix) {
		return false
	}
	name := strings.TrimPrefix(s, actionPrefix)
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '*' || r == '?':
		default:
			return false
		}
	}
	return true
}

// actionPatternKnown reports whether a pattern is "known": either it has no
// wildcards and is in KnownActions, or it has wildcards and could match at
// least one known action.
func actionPatternKnown(pattern string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.ContainsAny(pattern, "*?") {
		return KnownActions[pattern]
	}
	for a := range KnownActions {
		if matchGlob(pattern, a) {
			return true
		}
	}
	return false
}

// cut is strings.Cut, kept local so the package has no unnecessary coupling to
// the stdlib version's availability guarantees.
func cut(s string, sep byte) (before, after string, found bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
