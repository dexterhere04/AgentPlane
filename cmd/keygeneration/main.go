package main

import (
	"fmt"
	"log"

	"github.com/dexterhere04/AgentPlane/internal/api"
	"github.com/dexterhere04/AgentPlane/internal/config"
)

func main() {
	if err := config.ConfigureSecretStore(); err != nil {
		log.Fatal(err)
	}

	pepper, err := config.KeyPepper()
	if err != nil {
		log.Fatal(err)
	}

	key, err := api.GenerateAPIKey(pepper)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Generated API Key:")
	fmt.Println(key.FullKey)

	fmt.Println("\nKey ID:")
	fmt.Println(key.KeyID)

	fmt.Println("\nSecret Hash:")
	fmt.Println(key.SecretHash)
}
