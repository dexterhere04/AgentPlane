package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dexterhere04/AgentPlane/internal/proxy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RoutingAdmin struct {
	providers *proxy.ProviderStore
	rules     *proxy.RoutingRuleStore
	reload    func(context.Context) error
}

func NewRoutingAdmin(
	providers *proxy.ProviderStore,
	rules *proxy.RoutingRuleStore,
	reload func(context.Context) error,
) *RoutingAdmin {
	return &RoutingAdmin{
		providers: providers,
		rules:     rules,
		reload:    reload,
	}
}

type providerRequest struct {
	Name            string   `json:"name"`
	BaseURL         string   `json:"base_url"`
	SecretKey       string   `json:"secret_key,omitempty"`
	ProviderGroup   string   `json:"provider_group,omitempty"`
	Weight          int      `json:"weight"`
	SupportedModels []string `json:"supported_models"`
	TimeoutMS       int      `json:"timeout_ms"`
	MaxRetries      int      `json:"max_retries"`
	Enabled         bool     `json:"enabled"`
}

type routingRuleRequest struct {
	Name                  string          `json:"name"`
	Priority              int             `json:"priority"`
	ModelMatcher          json.RawMessage `json:"model_matcher,omitempty"`
	RoleMatcher           json.RawMessage `json:"role_matcher,omitempty"`
	UserMatcher           json.RawMessage `json:"user_matcher,omitempty"`
	Action                string          `json:"action"`
	ProviderGroup         string          `json:"provider_group,omitempty"`
	TimeoutRetryOverrides json.RawMessage `json:"timeout_retry_overrides,omitempty"`
	Enabled               bool            `json:"enabled"`
}

func (h *RoutingAdmin) ListProviders() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		records, err := h.providers.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, records)
	})
}

func (h *RoutingAdmin) GetProvider() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid provider id", http.StatusBadRequest)
			return
		}

		record, err := h.providers.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, proxy.ErrProviderNotFound) {
				http.Error(w, "provider not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, record)
	})
}

func (h *RoutingAdmin) CreateProvider() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req providerRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := validateProviderRequest(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		record := proxy.ProviderRecord{
			Name:            strings.TrimSpace(req.Name),
			BaseURL:         strings.TrimSpace(req.BaseURL),
			SecretRef:       strings.TrimSpace(req.SecretKey),
			ProviderGroup:   strings.TrimSpace(req.ProviderGroup),
			Weight:          req.Weight,
			SupportedModels: req.SupportedModels,
			TimeoutMS:       req.TimeoutMS,
			MaxRetries:      req.MaxRetries,
			Enabled:         req.Enabled,
		}

		created, err := h.providers.Create(r.Context(), record)
		if err != nil {
			if errors.Is(err, proxy.ErrDuplicateProvider) {
				http.Error(w, "provider already exists", http.StatusConflict)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("provider created but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusCreated, created)
	})
}

func (h *RoutingAdmin) UpdateProvider() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid provider id", http.StatusBadRequest)
			return
		}

		var req providerRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := validateProviderRequest(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		record := proxy.ProviderRecord{
			ID:              id,
			Name:            strings.TrimSpace(req.Name),
			BaseURL:         strings.TrimSpace(req.BaseURL),
			SecretRef:       strings.TrimSpace(req.SecretKey),
			ProviderGroup:   strings.TrimSpace(req.ProviderGroup),
			Weight:          req.Weight,
			SupportedModels: req.SupportedModels,
			TimeoutMS:       req.TimeoutMS,
			MaxRetries:      req.MaxRetries,
			Enabled:         req.Enabled,
		}

		updated, err := h.providers.Update(r.Context(), id, record)
		if err != nil {
			if errors.Is(err, proxy.ErrProviderNotFound) {
				http.Error(w, "provider not found", http.StatusNotFound)
				return
			}

			if errors.Is(err, proxy.ErrDuplicateProvider) {
				http.Error(w, "provider already exists", http.StatusConflict)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("provider updated but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusOK, updated)
	})
}

func (h *RoutingAdmin) EnableProvider() http.Handler {
	return h.setProviderEnabled(true)
}

func (h *RoutingAdmin) DisableProvider() http.Handler {
	return h.setProviderEnabled(false)
}

func (h *RoutingAdmin) setProviderEnabled(enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid provider id", http.StatusBadRequest)
			return
		}

		if err := h.providers.SetEnabled(r.Context(), id, enabled); err != nil {
			if errors.Is(err, proxy.ErrProviderNotFound) {
				http.Error(w, "provider not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("provider state updated but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": enabled,
		})
	})
}

func (h *RoutingAdmin) DeleteProvider() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid provider id", http.StatusBadRequest)
			return
		}

		if err := h.providers.Delete(r.Context(), id); err != nil {
			if errors.Is(err, proxy.ErrProviderNotFound) {
				http.Error(w, "provider not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("provider deleted but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

func (h *RoutingAdmin) ListRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rules, err := h.rules.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, rules)
	})
}

func (h *RoutingAdmin) GetRule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "invalid routing rule id", http.StatusBadRequest)
			return
		}

		rule, err := h.rules.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "routing rule not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, rule)
	})
}

func (h *RoutingAdmin) CreateRule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req routingRuleRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rule, err := routingRuleFromRequest(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		created, err := h.rules.Create(r.Context(), rule)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("routing rule created but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusCreated, created)
	})
}

func (h *RoutingAdmin) UpdateRule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "invalid routing rule id", http.StatusBadRequest)
			return
		}

		var req routingRuleRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rule, err := routingRuleFromRequest(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rule.ID = id

		updated, err := h.rules.Update(r.Context(), id, rule)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "routing rule not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("routing rule updated but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusOK, updated)
	})
}

func (h *RoutingAdmin) EnableRule() http.Handler {
	return h.setRuleEnabled(true)
}

func (h *RoutingAdmin) DisableRule() http.Handler {
	return h.setRuleEnabled(false)
}

func (h *RoutingAdmin) setRuleEnabled(enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "invalid routing rule id", http.StatusBadRequest)
			return
		}

		if err := h.rules.SetEnabled(r.Context(), id, enabled); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "routing rule not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("routing rule state updated but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": enabled,
		})
	})
}

func (h *RoutingAdmin) DeleteRule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "invalid routing rule id", http.StatusBadRequest)
			return
		}

		if err := h.rules.Delete(r.Context(), id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "routing rule not found", http.StatusNotFound)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := h.reload(r.Context()); err != nil {
			http.Error(
				w,
				fmt.Sprintf("routing rule deleted but reload failed: %v", err),
				http.StatusInternalServerError,
			)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

func validateProviderRequest(req providerRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}

	if strings.TrimSpace(req.BaseURL) == "" {
		return errors.New("base_url is required")
	}

	if req.Weight < 0 {
		return errors.New("weight must be >= 0")
	}

	if req.TimeoutMS <= 0 {
		return errors.New("timeout_ms must be > 0")
	}

	if req.MaxRetries < 0 {
		return errors.New("max_retries must be >= 0")
	}

	return nil
}

func routingRuleFromRequest(req routingRuleRequest) (proxy.RoutingRule, error) {
	rule := proxy.RoutingRule{
		Name:                  strings.TrimSpace(req.Name),
		Priority:              req.Priority,
		ModelMatcher:          req.ModelMatcher,
		RoleMatcher:           req.RoleMatcher,
		UserMatcher:           req.UserMatcher,
		Action:                strings.TrimSpace(req.Action),
		ProviderGroup:         strings.TrimSpace(req.ProviderGroup),
		TimeoutRetryOverrides: req.TimeoutRetryOverrides,
		Enabled:               req.Enabled,
	}

	if rule.Name == "" {
		return proxy.RoutingRule{}, errors.New("name is required")
	}

	if rule.Action == "" {
		return proxy.RoutingRule{}, errors.New("action is required")
	}

	if _, err := proxy.NewRuleSet([]proxy.RoutingRule{rule}); err != nil {
		return proxy.RoutingRule{}, fmt.Errorf("invalid routing rule: %w", err)
	}

	if _, err := rule.ResilienceConfig(); err != nil {
		return proxy.RoutingRule{}, fmt.Errorf("invalid resilience configuration: %w", err)
	}

	return rule, nil
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(r.PathValue(name)))
}
