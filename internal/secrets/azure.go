package secrets

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

type AzureKeyVaultStore struct {
	VaultURL string
}

func (s *AzureKeyVaultStore) GetSecret(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", fmt.Errorf("azure: creating credential: %w", err)
	}

	client, err := azsecrets.NewClient(s.VaultURL, cred, nil)
	if err != nil {
		return "", fmt.Errorf("azure: creating client: %w", err)
	}

	resp, err := client.GetSecret(ctx, key, "", nil)
	if err != nil {
		return "", fmt.Errorf("azure: getting secret %q: %w", key, err)
	}

	if resp.Value == nil {
		return "", fmt.Errorf("azure: secret %q has no value", key)
	}

	return *resp.Value, nil
}
