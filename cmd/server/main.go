package main

import (
	"log"
	"net/http"
)

func main() {
	log.Println("Starting AgentPlan on :3001")
	err := http.ListenAndServe(":3001", nil)
	if err != nil {
		log.Fatal(err)
	}
}
