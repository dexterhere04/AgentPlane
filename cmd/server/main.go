package main

import (
	"log"
	"net/http"
	"os"

	"github.com/dexterhere04/AgentPlane/internal/dashboard"
	"github.com/dexterhere04/AgentPlane/internal/handlers"
	"github.com/dexterhere04/AgentPlane/internal/observability"
)

func main() {
	bus := observability.DefaultBus

	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/chat", handlers.Chat)
	mux.HandleFunc("/events", observability.SSEHandler(bus))
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboard.HTML))
	})

	log.Printf("AgentPlane Dev Mode")
	log.Printf("  Gateway   → http://localhost:%s/chat", port)
	log.Printf("  Events    → http://localhost:%s/events", port)
	log.Printf("  Dashboard → http://localhost:%s/dashboard", port)
	log.Printf("  Mock API  → http://localhost:%s (set OPENAI_BASE_URL)", port)

	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal(err)
	}
}
