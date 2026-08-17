package config

import (
	"fmt"
	"os"
)

const adminTokenEnvVar = "AGENTPLANE_ADMIN_TOKEN"

// AdminToken returns the token used to authorize administrative
// management endpoints.
func AdminToken() (string, error) {
	token := os.Getenv(adminTokenEnvVar)
	if token == "" {
		return "", fmt.Errorf("config: %s environment variable is not set", adminTokenEnvVar)
	}

	return token, nil
}
