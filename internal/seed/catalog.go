// Package seed populates AgentPlane with a realistic, self-consistent
// organization for demos: IAM policies/roles/groups, users with API keys,
// providers and routing rules in Postgres, plus a matching window of
// observability telemetry in ClickHouse.
//
// Everything here is deterministic given a RNG seed, so a demo can be
// regenerated identically. See cmd/seed for the command-line entry point.
package seed

import (
	"encoding/json"

	"github.com/dexterhere04/AgentPlane/internal/iam"
)

// Organization identity used across Postgres records and ClickHouse telemetry.
const (
	CompanyName = "Northwind AI"
	OrgID       = "northwind-ai"
	EmailDomain = "northwindai.example"
)

// Projects attributed to traffic in the analytics store.
var Projects = []string{
	"proj-copilot",
	"proj-support-bot",
	"proj-research-eval",
	"proj-docs-assistant",
	"proj-data-pipeline",
}

// policySeed is a named IAM policy document created for the demo org.
type policySeed struct {
	Name        string
	Description string
	Statements  []iam.Statement
}

// roleSeed is an assumable identity carrying attached policies.
type roleSeed struct {
	Name           string
	Description    string
	Policies       []string
	BoundaryPolicy string // optional permission boundary policy name
}

// groupSeed is a team mapped to an IAM group with one granted role.
type groupSeed struct {
	Name        string
	Description string
	Role        string
	Policies    []string // policies attached directly to the group
}

// userSeed is a demo identity. Team is the IAM group it belongs to.
type userSeed struct {
	Username       string
	Email          string
	Team           string
	DirectRoles    []string
	Policies       []string // policies attached directly to the user
	BoundaryPolicy string   // optional permission boundary policy name
	TrafficWeight  int      // relative analytics traffic (0 = no traffic)
	EU             bool     // subject to EU data-residency routing
	ServiceAccount bool
}

// providerSeed is an upstream provider record.
type providerSeed struct {
	Name                    string
	Group                   string
	Weight                  int
	Models                  []string
	TimeoutMS               int
	MaxRetries              int
	BreakerFailureThreshold int
	Enabled                 bool
}

// ruleSeed is a routing rule.
type ruleSeed struct {
	Name          string
	Priority      int
	ModelMatcher  json.RawMessage
	RoleMatcher   json.RawMessage
	UserMatcher   json.RawMessage
	Action        string
	ProviderGroup string
	OverrideJSON  json.RawMessage
	Enabled       bool
}

// modelSpec describes one model's traffic characteristics for analytics.
type modelSpec struct {
	Name          string
	Provider      string // primary upstream
	Group         string
	Route         string
	Backup        string // failover upstream (may be empty)
	InPrice       float64
	OutPrice      float64
	LatencyMS     int
	InTokens      int
	OutTokens     int
	TrafficWeight int
}

// ---------------------------------------------------------------------------
// IAM policies
// ---------------------------------------------------------------------------

func customPolicies() []policySeed {
	allow := iam.EffectAllow
	deny := iam.EffectDeny

	return []policySeed{
		{
			Name:        "APTeamLead",
			Description: "Team leads: invoke models plus read-only operational visibility.",
			Statements: []iam.Statement{
				{Sid: "Invoke", Effect: allow,
					Action:   iam.StringOrSlice{iam.ActionInvokeChat, iam.ActionInvokeModel},
					Resource: iam.StringOrSlice{"arn:ap:*"}},
				{Sid: "OperationalRead", Effect: allow,
					Action: iam.StringOrSlice{
						iam.ActionListProviders, iam.ActionListRoutingRules, iam.ActionViewMetrics,
						iam.ActionListUsers, iam.ActionListApiKeys,
					},
					Resource: iam.StringOrSlice{"arn:ap:*"}},
			},
		},
		{
			Name:        "APServiceAccount",
			Description: "Non-human automation: invoke chat and models only, no administration.",
			Statements: []iam.Statement{
				{Sid: "InvokeChat", Effect: allow,
					Action:   iam.StringOrSlice{iam.ActionInvokeChat},
					Resource: iam.StringOrSlice{"*"}},
				{Sid: "InvokeAnyModel", Effect: allow,
					Action:   iam.StringOrSlice{iam.ActionInvokeModel},
					Resource: iam.StringOrSlice{"arn:ap:model/*"}},
			},
		},
		{
			Name:        "APAuditor",
			Description: "Security/compliance read-only access across configuration and IAM.",
			Statements: []iam.Statement{
				{Sid: "ReadEverything", Effect: allow,
					Action: iam.StringOrSlice{
						iam.ActionListProviders, iam.ActionListRoutingRules, iam.ActionViewMetrics,
						iam.ActionListUsers, iam.ActionListApiKeys, iam.ActionListPolicies,
						iam.ActionGetPolicy, iam.ActionListGroups, iam.ActionGetGroup,
						iam.ActionListRoles, iam.ActionGetRole,
					},
					Resource: iam.StringOrSlice{"arn:ap:*"}},
			},
		},
		{
			Name:        "APNoAdminBoundary",
			Description: "Permission-boundary ceiling: invoke-only. Restricts, never grants.",
			Statements: []iam.Statement{
				{Sid: "InvokeOnly", Effect: allow,
					Action: iam.StringOrSlice{
						iam.ActionInvokeChat, iam.ActionInvokeModel,
						iam.ActionListProviders, iam.ActionListRoutingRules, iam.ActionViewMetrics,
					},
					Resource: iam.StringOrSlice{"arn:ap:*"}},
			},
		},
		{
			Name:        "APDenyExperimental",
			Description: "Explicit deny: block preview/experimental model endpoints org-wide.",
			Statements: []iam.Statement{
				{Sid: "DenyExperimentalModels", Effect: deny,
					Action: iam.StringOrSlice{iam.ActionInvokeModel},
					Resource: iam.StringOrSlice{
						"arn:ap:model/*-experimental",
						"arn:ap:model/*-preview",
						"arn:ap:model/o1-preview",
						"arn:ap:model/claude-opus-preview",
					}},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Roles and groups
// ---------------------------------------------------------------------------

func demoRoles() []roleSeed {
	return []roleSeed{
		{Name: "team-lead", Description: "Engineering/analytics team leads.", Policies: []string{"APTeamLead"}},
		{Name: "service-account", Description: "Non-human automation principals.", Policies: []string{"APServiceAccount"}},
		{Name: "readonly", Description: "Read-only operational access.", Policies: []string{"APReadOnly"}},
		{Name: "security-auditor", Description: "Security and compliance auditors.", Policies: []string{"APAuditor", "APReadOnly"}},
		{Name: "eu-member", Description: "Members whose traffic must stay in the EU region.", Policies: []string{"APMember"}},
	}
}

func demoGroups() []groupSeed {
	return []groupSeed{
		{Name: "platform", Description: "Platform Engineering", Role: "admin"},
		{Name: "ml-research", Description: "Machine Learning Research", Role: "member"},
		{Name: "app-eng", Description: "Application Engineering", Role: "member"},
		{Name: "support", Description: "Customer Support", Role: "member"},
		{Name: "analytics", Description: "Data & Analytics", Role: "readonly"},
		{Name: "security", Description: "Security & Compliance", Role: "security-auditor"},
		{Name: "finance", Description: "Finance & Operations", Role: "readonly"},
		{Name: "service-accounts", Description: "Non-human service accounts", Role: "service-account"},
	}
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func demoUsers() []userSeed {
	email := func(local string) string { return local + "@" + EmailDomain }

	return []userSeed{
		// Platform Engineering.
		{Username: "ava.chen", Email: email("ava.chen"), Team: "platform", TrafficWeight: 6},
		{Username: "marcus.webb", Email: email("marcus.webb"), Team: "platform", TrafficWeight: 4},
		{Username: "priya.nair", Email: email("priya.nair"), Team: "platform", TrafficWeight: 5},
		{Username: "kenji.sato", Email: email("kenji.sato"), Team: "platform", TrafficWeight: 4},

		// ML Research.
		{Username: "elena.rossi", Email: email("elena.rossi"), Team: "ml-research", DirectRoles: []string{"team-lead"}, TrafficWeight: 9},
		{Username: "tom.okafor", Email: email("tom.okafor"), Team: "ml-research", TrafficWeight: 4},
		{Username: "yuki.tanaka", Email: email("yuki.tanaka"), Team: "ml-research", TrafficWeight: 5},
		{Username: "sara.lindqvist", Email: email("sara.lindqvist"), Team: "ml-research", DirectRoles: []string{"eu-member"}, TrafficWeight: 3, EU: true},

		// Application Engineering.
		{Username: "diego.alvarez", Email: email("diego.alvarez"), Team: "app-eng", DirectRoles: []string{"team-lead"}, TrafficWeight: 7},
		{Username: "hannah.kim", Email: email("hannah.kim"), Team: "app-eng", TrafficWeight: 6},
		{Username: "bhavna.patel", Email: email("bhavna.patel"), Team: "app-eng", TrafficWeight: 5},
		{Username: "lucas.meyer", Email: email("lucas.meyer"), Team: "app-eng", DirectRoles: []string{"eu-member"}, TrafficWeight: 4, EU: true},
		{Username: "jonas.berg", Email: email("jonas.berg"), Team: "app-eng", DirectRoles: []string{"eu-member"}, TrafficWeight: 3, EU: true},

		// Customer Support.
		{Username: "grace.osei", Email: email("grace.osei"), Team: "support", DirectRoles: []string{"team-lead"}, TrafficWeight: 5},
		{Username: "noah.bennett", Email: email("noah.bennett"), Team: "support", TrafficWeight: 4},
		{Username: "mei.wong", Email: email("mei.wong"), Team: "support", TrafficWeight: 4},

		// Data & Analytics.
		{Username: "omar.haddad", Email: email("omar.haddad"), Team: "analytics", DirectRoles: []string{"team-lead"}, TrafficWeight: 6},
		{Username: "chloe.dubois", Email: email("chloe.dubois"), Team: "analytics", DirectRoles: []string{"eu-member"}, TrafficWeight: 4, EU: true},

		// Security & Compliance.
		{Username: "ivan.petrov", Email: email("ivan.petrov"), Team: "security", TrafficWeight: 3},
		{Username: "fatima.alsayed", Email: email("fatima.alsayed"), Team: "security", TrafficWeight: 2},

		// Finance & Operations.
		{Username: "robert.chen", Email: email("robert.chen"), Team: "finance", TrafficWeight: 2},
		{Username: "anika.sharma", Email: email("anika.sharma"), Team: "finance", TrafficWeight: 2},

		// Non-human service accounts (invoke-only boundary).
		{Username: "svc-batch-inference", Email: email("svc-batch-inference"), Team: "service-accounts", BoundaryPolicy: "APNoAdminBoundary", TrafficWeight: 30, ServiceAccount: true},
		{Username: "svc-support-bot", Email: email("svc-support-bot"), Team: "service-accounts", BoundaryPolicy: "APNoAdminBoundary", TrafficWeight: 22, ServiceAccount: true},
		{Username: "svc-eval-harness", Email: email("svc-eval-harness"), Team: "service-accounts", BoundaryPolicy: "APNoAdminBoundary", TrafficWeight: 14, ServiceAccount: true},
		{Username: "svc-docs-rag", Email: email("svc-docs-rag"), Team: "service-accounts", BoundaryPolicy: "APNoAdminBoundary", TrafficWeight: 10, ServiceAccount: true},
	}
}

// ---------------------------------------------------------------------------
// Providers and models
// ---------------------------------------------------------------------------

func demoProviders() []providerSeed {
	return []providerSeed{
		{Name: "openai-primary", Group: "primary", Weight: 100,
			Models:    []string{"gpt-4o", "gpt-4o-mini", "gpt-4.1", "gpt-4.1-mini", "o3-mini"},
			TimeoutMS: 30000, MaxRetries: 1, BreakerFailureThreshold: 5, Enabled: true},
		{Name: "openai-backup", Group: "primary", Weight: 30,
			Models:    []string{"gpt-4o", "gpt-4o-mini", "gpt-4.1"},
			TimeoutMS: 45000, MaxRetries: 1, BreakerFailureThreshold: 3, Enabled: true},
		{Name: "azure-openai-eu", Group: "eu", Weight: 60,
			Models:    []string{"gpt-4o", "gpt-4o-mini"},
			TimeoutMS: 30000, MaxRetries: 1, BreakerFailureThreshold: 4, Enabled: true},
		{Name: "anthropic-claude", Group: "anthropic", Weight: 90,
			Models:    []string{"claude-3-7-sonnet", "claude-3-5-sonnet", "claude-3-5-haiku"},
			TimeoutMS: 60000, MaxRetries: 1, BreakerFailureThreshold: 5, Enabled: true},
		{Name: "gcp-vertex-gemini", Group: "google", Weight: 70,
			Models:    []string{"gemini-2.0-flash", "gemini-1.5-pro"},
			TimeoutMS: 45000, MaxRetries: 1, BreakerFailureThreshold: 4, Enabled: true},
		{Name: "mistral-eu", Group: "oss", Weight: 40,
			Models:    []string{"mistral-large-latest", "mistral-small-latest"},
			TimeoutMS: 30000, MaxRetries: 1, BreakerFailureThreshold: 3, Enabled: true},
		{Name: "groq-fast", Group: "fast", Weight: 80,
			Models:    []string{"llama-3.3-70b-versatile", "mixtral-8x7b-32768"},
			TimeoutMS: 15000, MaxRetries: 2, BreakerFailureThreshold: 5, Enabled: true},
		{Name: "openrouter-pool", Group: "fallback", Weight: 20,
			Models:    []string{"*"},
			TimeoutMS: 60000, MaxRetries: 2, BreakerFailureThreshold: 6, Enabled: true},
		{Name: "selfhost-vllm", Group: "onprem", Weight: 30,
			Models:    []string{"llama-3.1-70b-instruct", "qwen2.5-72b-instruct"},
			TimeoutMS: 120000, MaxRetries: 0, BreakerFailureThreshold: 2, Enabled: true},
		{Name: "bedrock-titan-dr", Group: "dr", Weight: 0,
			Models:    []string{"amazon.titan-text-premier-v1"},
			TimeoutMS: 60000, MaxRetries: 1, BreakerFailureThreshold: 5, Enabled: false},
	}
}

// demoModels drives analytics generation. Traffic weights approximate a real
// mix: cheap fast models dominate volume, frontier models carry the cost.
func demoModels() []modelSpec {
	return []modelSpec{
		{Name: "gpt-4o", Provider: "openai-primary", Backup: "openai-backup", Group: "primary", Route: "gpt-primary",
			InPrice: 2.50, OutPrice: 10.00, LatencyMS: 1400, InTokens: 900, OutTokens: 420, TrafficWeight: 26},
		{Name: "gpt-4o-mini", Provider: "openai-primary", Backup: "openai-backup", Group: "primary", Route: "gpt-primary",
			InPrice: 0.15, OutPrice: 0.60, LatencyMS: 650, InTokens: 700, OutTokens: 300, TrafficWeight: 20},
		{Name: "gpt-4.1", Provider: "openai-primary", Backup: "openai-backup", Group: "primary", Route: "gpt-primary",
			InPrice: 2.00, OutPrice: 8.00, LatencyMS: 1500, InTokens: 1200, OutTokens: 500, TrafficWeight: 7},
		{Name: "o3-mini", Provider: "openai-primary", Backup: "openai-backup", Group: "primary", Route: "gpt-primary",
			InPrice: 1.10, OutPrice: 4.40, LatencyMS: 3200, InTokens: 1500, OutTokens: 1100, TrafficWeight: 4},
		{Name: "claude-3-7-sonnet", Provider: "anthropic-claude", Group: "anthropic", Route: "claude-workloads",
			InPrice: 3.00, OutPrice: 15.00, LatencyMS: 1800, InTokens: 1600, OutTokens: 700, TrafficWeight: 10},
		{Name: "claude-3-5-sonnet", Provider: "anthropic-claude", Group: "anthropic", Route: "claude-workloads",
			InPrice: 3.00, OutPrice: 15.00, LatencyMS: 1700, InTokens: 1500, OutTokens: 650, TrafficWeight: 5},
		{Name: "claude-3-5-haiku", Provider: "anthropic-claude", Group: "anthropic", Route: "claude-workloads",
			InPrice: 0.80, OutPrice: 4.00, LatencyMS: 700, InTokens: 800, OutTokens: 320, TrafficWeight: 6},
		{Name: "gemini-2.0-flash", Provider: "gcp-vertex-gemini", Group: "google", Route: "gemini-workloads",
			InPrice: 0.10, OutPrice: 0.40, LatencyMS: 480, InTokens: 1000, OutTokens: 380, TrafficWeight: 8},
		{Name: "gemini-1.5-pro", Provider: "gcp-vertex-gemini", Group: "google", Route: "gemini-workloads",
			InPrice: 1.25, OutPrice: 5.00, LatencyMS: 1300, InTokens: 2000, OutTokens: 600, TrafficWeight: 3},
		{Name: "mistral-large-latest", Provider: "mistral-eu", Group: "oss", Route: "open-models",
			InPrice: 2.00, OutPrice: 6.00, LatencyMS: 1200, InTokens: 900, OutTokens: 400, TrafficWeight: 2},
		{Name: "mistral-small-latest", Provider: "mistral-eu", Group: "oss", Route: "open-models",
			InPrice: 0.20, OutPrice: 0.60, LatencyMS: 520, InTokens: 700, OutTokens: 280, TrafficWeight: 3},
		{Name: "llama-3.3-70b-versatile", Provider: "groq-fast", Group: "fast", Route: "fast-inference",
			InPrice: 0.59, OutPrice: 0.79, LatencyMS: 260, InTokens: 800, OutTokens: 340, TrafficWeight: 4},
		{Name: "mixtral-8x7b-32768", Provider: "groq-fast", Group: "fast", Route: "fast-inference",
			InPrice: 0.24, OutPrice: 0.24, LatencyMS: 210, InTokens: 900, OutTokens: 300, TrafficWeight: 2},
		{Name: "llama-3.1-70b-instruct", Provider: "selfhost-vllm", Group: "onprem", Route: "batch-inference-onprem",
			InPrice: 0.35, OutPrice: 0.45, LatencyMS: 900, InTokens: 1400, OutTokens: 500, TrafficWeight: 1},
	}
}

// ---------------------------------------------------------------------------
// Routing rules
// ---------------------------------------------------------------------------

func demoRules() []ruleSeed {
	return []ruleSeed{
		{
			Name: "block-preview-models", Priority: 1, Action: "force_deny",
			ModelMatcher: mustJSON(map[string]any{"in": []string{
				"gpt-4o-experimental", "o1-preview", "claude-opus-preview",
			}}),
			Enabled: true,
		},
		{
			Name: "eu-data-residency", Priority: 10, Action: "route", ProviderGroup: "eu",
			RoleMatcher:  mustJSON(map[string]any{"in": []string{"eu-member", "eu-data-owner"}}),
			ModelMatcher: mustJSON(map[string]any{"prefix": "gpt-"}),
			Enabled:      true,
		},
		{
			Name: "claude-workloads", Priority: 20, Action: "route", ProviderGroup: "anthropic",
			ModelMatcher: mustJSON(map[string]any{"prefix": "claude-"}),
			Enabled:      true,
		},
		{
			Name: "gemini-workloads", Priority: 30, Action: "route", ProviderGroup: "google",
			ModelMatcher: mustJSON(map[string]any{"prefix": "gemini-"}),
			Enabled:      true,
		},
		{
			Name: "fast-inference", Priority: 40, Action: "weighted", ProviderGroup: "fast",
			ModelMatcher: mustJSON(map[string]any{"in": []string{"llama-3.3-70b-versatile", "mixtral-8x7b-32768"}}),
			Enabled:      true,
		},
		{
			Name: "open-models", Priority: 50, Action: "weighted", ProviderGroup: "oss",
			ModelMatcher: mustJSON(map[string]any{"prefix": "mistral-"}),
			Enabled:      true,
		},
		{
			Name: "batch-inference-onprem", Priority: 60, Action: "route", ProviderGroup: "onprem",
			UserMatcher: mustJSON(map[string]any{"usernames": []string{"svc-batch-inference"}}),
			Enabled:     true,
		},
		{
			Name: "gpt-primary", Priority: 70, Action: "weighted", ProviderGroup: "primary",
			ModelMatcher: mustJSON(map[string]any{"prefix": "gpt-"}),
			OverrideJSON: mustJSON(map[string]any{"max_retries": 2, "backoff_base_ms": 150, "jitter_percent": 20}),
			Enabled:      true,
		},
		{
			Name: "default-pool", Priority: 90, Action: "weighted", ProviderGroup: "fallback",
			Enabled: true,
		},
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
