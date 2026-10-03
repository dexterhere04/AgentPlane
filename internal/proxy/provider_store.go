package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProviderStore struct {
	pool *pgxpool.Pool
}

func NewProviderStore(pool *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{pool: pool}
}

type providerRow struct {
	Name            string
	BaseURL         string
	SecretRef       string
	ProviderGroup   string
	Weight          int
	SupportedModels []string
	TimeoutMS       int
	MaxRetries      int
	Enabled         bool
}

func (s *ProviderStore) ListEnabled(ctx context.Context) ([]*Upstream, error) {
	const query = `
		SELECT
			name,
			base_url,
			COALESCE(secret_ref, ''),
			COALESCE(provider_group, ''),
			weight,
			supported_models,
			timeout_ms,
			max_retries,
			enabled
		FROM providers
		WHERE enabled = TRUE
		ORDER BY name ASC
	`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	defer rows.Close()

	var upstreams []*Upstream

	for rows.Next() {
		var row providerRow
		var supportedModels []byte

		if err := rows.Scan(
			&row.Name,
			&row.BaseURL,
			&row.SecretRef,
			&row.ProviderGroup,
			&row.Weight,
			&supportedModels,
			&row.TimeoutMS,
			&row.MaxRetries,
			&row.Enabled,
		); err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}

		if err := json.Unmarshal(supportedModels, &row.SupportedModels); err != nil {
			return nil, fmt.Errorf(
				"decode supported_models for provider %q: %w",
				row.Name,
				err,
			)
		}

		secretRef := row.SecretRef
		if secretRef == "" {
			secretRef = "OPENAI_API_KEY"
		}

		apiKey, err := config.Secret(secretRef)
		if err != nil {
			return nil, fmt.Errorf(
				"load secret %q for provider %q: %w",
				secretRef,
				row.Name,
				err,
			)
		}

		provider := NewOpenAIProvider(apiKey, row.BaseURL)

		upstream := NewUpstream(
			row.Name,
			provider,
			WithProviderGroup(row.ProviderGroup),
			WithWeight(row.Weight),
			WithModels(row.SupportedModels...),
			WithTimeout(time.Duration(row.TimeoutMS)*time.Millisecond),
			WithMaxRetries(row.MaxRetries),
		)

		upstreams = append(upstreams, upstream)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate providers: %w", err)
	}

	return upstreams, nil
}
