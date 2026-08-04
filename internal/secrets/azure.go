package secrets

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

type AzureKeyVaultStore struct {
	VaultURL string

	client   *azsecrets.Client
	initOnce sync.Once
	initErr  error
}

func (s *AzureKeyVaultStore) GetSecret(key string) (string, error) {
	s.initOnce.Do(func() {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			s.initErr = fmt.Errorf("azure: creating credential: %w", err)
			return
		}

		client, err := azsecrets.NewClient(s.VaultURL, cred, nil)
		if err != nil {
			s.initErr = fmt.Errorf("azure: creating client: %w", err)
			return
		}
		s.client = client
	})
	if s.initErr != nil {
		return "", s.initErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := s.client.GetSecret(ctx, key, "", nil)
	if err != nil {
		return "", fmt.Errorf("azure: getting secret %q: %w", key, err)
	}

	if resp.Value == nil {
		return "", fmt.Errorf("azure: secret %q has no value", key)
	}

	return *resp.Value, nil
}
