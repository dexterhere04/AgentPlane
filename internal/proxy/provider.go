package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dexterhere04/AgentPlane/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Provider interface {
	Name() string
	Forward(ctx context.Context, body []byte, requestID string) ([]byte, error)
}

var (
	ErrProviderNotFound  = errors.New("provider not found")
	ErrDuplicateProvider = errors.New("provider already exists")
)

type ProviderStore struct {
	pool *pgxpool.Pool
}

func NewProviderStore(pool *pgxpool.Pool) *ProviderStore {
	return &ProviderStore{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type ProviderRecord struct {
	ID              uuid.UUID
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

type providerRow struct {
	ID              uuid.UUID
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

func (s *ProviderStore) Get(ctx context.Context, id uuid.UUID) (ProviderRecord, error) {
	const query = `
		SELECT
			id,
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
		WHERE id = $1
	`

	var row providerRow
	var supportedModels []byte

	err := s.pool.QueryRow(ctx, query, id).Scan(
		&row.ID,
		&row.Name,
		&row.BaseURL,
		&row.SecretRef,
		&row.ProviderGroup,
		&row.Weight,
		&supportedModels,
		&row.TimeoutMS,
		&row.MaxRetries,
		&row.Enabled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderRecord{}, ErrProviderNotFound
	}
	if err != nil {
		return ProviderRecord{}, fmt.Errorf("get provider: %w", err)
	}

	if err := json.Unmarshal(supportedModels, &row.SupportedModels); err != nil {
		return ProviderRecord{}, fmt.Errorf("decode supported_models: %w", err)
	}

	return providerRecord(row), nil
}

func (s *ProviderStore) List(ctx context.Context) ([]ProviderRecord, error) {
	const query = `
		SELECT
			id,
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
		ORDER BY name ASC
	`

	return s.queryRecords(ctx, query)
}

func (s *ProviderStore) Create(
	ctx context.Context,
	record ProviderRecord,
) (ProviderRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}

	models, err := json.Marshal(record.SupportedModels)
	if err != nil {
		return ProviderRecord{}, fmt.Errorf("encode supported_models: %w", err)
	}

	const query = `
		INSERT INTO providers (
			id,
			name,
			base_url,
			secret_ref,
			provider_group,
			weight,
			supported_models,
			timeout_ms,
			max_retries,
			enabled
		)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10)
		RETURNING
			id,
			name,
			base_url,
			COALESCE(secret_ref, ''),
			COALESCE(provider_group, ''),
			weight,
			supported_models,
			timeout_ms,
			max_retries,
			enabled
	`

	var row providerRow
	var supportedModels []byte

	err = s.pool.QueryRow(
		ctx,
		query,
		record.ID,
		strings.TrimSpace(record.Name),
		strings.TrimSpace(record.BaseURL),
		strings.TrimSpace(record.SecretRef),
		strings.TrimSpace(record.ProviderGroup),
		record.Weight,
		models,
		record.TimeoutMS,
		record.MaxRetries,
		record.Enabled,
	).Scan(
		&row.ID,
		&row.Name,
		&row.BaseURL,
		&row.SecretRef,
		&row.ProviderGroup,
		&row.Weight,
		&supportedModels,
		&row.TimeoutMS,
		&row.MaxRetries,
		&row.Enabled,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ProviderRecord{}, ErrDuplicateProvider
		}
		return ProviderRecord{}, fmt.Errorf("create provider: %w", err)
	}

	if err := json.Unmarshal(supportedModels, &row.SupportedModels); err != nil {
		return ProviderRecord{}, fmt.Errorf("decode supported_models: %w", err)
	}

	return providerRecord(row), nil
}

func (s *ProviderStore) Update(
	ctx context.Context,
	id uuid.UUID,
	record ProviderRecord,
) (ProviderRecord, error) {
	models, err := json.Marshal(record.SupportedModels)
	if err != nil {
		return ProviderRecord{}, fmt.Errorf("encode supported_models: %w", err)
	}

	const query = `
		UPDATE providers
		SET
			name = $2,
			base_url = $3,
			secret_ref = NULLIF($4,''),
			provider_group = NULLIF($5,''),
			weight = $6,
			supported_models = $7,
			timeout_ms = $8,
			max_retries = $9,
			enabled = $10,
			updated_at = now()
		WHERE id = $1
		RETURNING
			id,
			name,
			base_url,
			COALESCE(secret_ref, ''),
			COALESCE(provider_group, ''),
			weight,
			supported_models,
			timeout_ms,
			max_retries,
			enabled
	`

	var row providerRow
	var supportedModels []byte

	err = s.pool.QueryRow(
		ctx,
		query,
		id,
		strings.TrimSpace(record.Name),
		strings.TrimSpace(record.BaseURL),
		strings.TrimSpace(record.SecretRef),
		strings.TrimSpace(record.ProviderGroup),
		record.Weight,
		models,
		record.TimeoutMS,
		record.MaxRetries,
		record.Enabled,
	).Scan(
		&row.ID,
		&row.Name,
		&row.BaseURL,
		&row.SecretRef,
		&row.ProviderGroup,
		&row.Weight,
		&supportedModels,
		&row.TimeoutMS,
		&row.MaxRetries,
		&row.Enabled,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderRecord{}, ErrProviderNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return ProviderRecord{}, ErrDuplicateProvider
		}
		return ProviderRecord{}, fmt.Errorf("update provider: %w", err)
	}

	if err := json.Unmarshal(supportedModels, &row.SupportedModels); err != nil {
		return ProviderRecord{}, fmt.Errorf("decode supported_models: %w", err)
	}

	return providerRecord(row), nil
}

func (s *ProviderStore) SetEnabled(
	ctx context.Context,
	id uuid.UUID,
	enabled bool,
) error {
	const query = `
		UPDATE providers
		SET enabled = $2, updated_at = now()
		WHERE id = $1
	`

	tag, err := s.pool.Exec(ctx, query, id, enabled)
	if err != nil {
		return fmt.Errorf("set provider enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderNotFound
	}
	return nil
}

func (s *ProviderStore) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM providers WHERE id = $1`

	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderNotFound
	}
	return nil
}

func (s *ProviderStore) ListEnabled(ctx context.Context) ([]*Upstream, error) {
	const query = `
		SELECT
			id,
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

	return s.loadUpstreams(ctx, query)
}

func (s *ProviderStore) queryRecords(
	ctx context.Context,
	query string,
	args ...any,
) ([]ProviderRecord, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	defer rows.Close()

	var records []ProviderRecord

	for rows.Next() {
		var row providerRow
		var supportedModels []byte

		if err := rows.Scan(
			&row.ID,
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
			return nil, fmt.Errorf("decode supported_models: %w", err)
		}

		records = append(records, providerRecord(row))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate providers: %w", err)
	}

	return records, nil
}

func (s *ProviderStore) loadUpstreams(
	ctx context.Context,
	query string,
) ([]*Upstream, error) {
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
			&row.ID,
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

func providerRecord(row providerRow) ProviderRecord {
	return ProviderRecord{
		ID:              row.ID,
		Name:            row.Name,
		BaseURL:         row.BaseURL,
		SecretRef:       row.SecretRef,
		ProviderGroup:   row.ProviderGroup,
		Weight:          row.Weight,
		SupportedModels: row.SupportedModels,
		TimeoutMS:       row.TimeoutMS,
		MaxRetries:      row.MaxRetries,
		Enabled:         row.Enabled,
	}
}
