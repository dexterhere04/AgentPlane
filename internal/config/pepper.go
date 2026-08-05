package config

import "fmt"

const apiKeyPepperEnvVar = "API_KEY_PEPPER"

func KeyPepper() (string, error) {
	pepper, err := Store.GetSecret(apiKeyPepperEnvVar)
	if err != nil {
		return "", fmt.Errorf("config: failed loading API key pepper: %w", err)
	}

	if pepper == "" {
		return "", fmt.Errorf("config: API_KEY_PEPPER is empty")
	}

	return pepper, nil
}