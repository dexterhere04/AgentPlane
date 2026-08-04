package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type AWSSecretsManagerStore struct {
	Region      string
	EndpointURL string
	SecretID    string
	JSONKey     string

	client   *secretsmanager.Client
	initOnce sync.Once
	initErr  error
}

func (s *AWSSecretsManagerStore) GetSecret(key string) (string, error) {
	s.initOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(s.Region))
		if err != nil {
			s.initErr = fmt.Errorf("aws: loading config: %w", err)
			return
		}

		opts := func(o *secretsmanager.Options) {}
		if s.EndpointURL != "" {
			opts = func(o *secretsmanager.Options) {
				o.BaseEndpoint = &s.EndpointURL
			}
		}

		s.client = secretsmanager.NewFromConfig(cfg, opts)
	})
	if s.initErr != nil {
		return "", s.initErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	input := &secretsmanager.GetSecretValueInput{
		SecretId: &s.SecretID,
	}

	output, err := s.client.GetSecretValue(ctx, input)
	if err != nil {
		return "", fmt.Errorf("aws: getting secret %q: %w", s.SecretID, err)
	}

	if output.SecretString == nil {
		return "", fmt.Errorf("aws: secret %q has no SecretString", s.SecretID)
	}

	if s.JSONKey == "" {
		return *output.SecretString, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(*output.SecretString), &data); err != nil {
		return "", fmt.Errorf("aws: parsing secret as JSON: %w", err)
	}

	val, ok := data[s.JSONKey]
	if !ok || val == nil {
		return "", fmt.Errorf("aws: key %q not found in secret %q", s.JSONKey, s.SecretID)
	}

	strVal, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("aws: key %q in secret %q is not a string", s.JSONKey, s.SecretID)
	}

	return strVal, nil
}
