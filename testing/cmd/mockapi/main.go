package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/dexterhere04/AgentPlane/testing/internal/faker"
)

func main() {
	port := env("MOCK_PORT", "3002")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChat)
	mux.HandleFunc("/health", handleHealth)

	log.Printf("Mock API listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := readBody(r)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if !json.Valid(body) {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	streaming := shouldStream(body)

	log.Printf("Incoming | stream=%v | body=%s", streaming, truncate(string(body), 120))

	if streaming {
		handleStream(w, r, body)
	} else {
		handleNormal(w, body)
	}
}

func handleNormal(w http.ResponseWriter, body []byte) {
	resp, err := faker.GenerateResponse(body)
	if err != nil {
		log.Printf("Error generating response: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("Response | %s", truncate(string(resp), 120))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

func handleStream(w http.ResponseWriter, r *http.Request, body []byte) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)

	chunks := make(chan []byte, 64)
	done := make(chan struct{})

	if err := faker.StreamResponse(body, chunks, done); err != nil {
		log.Printf("Stream error: %v", err)
		return
	}

	wordCount := 0
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			wordCount++
			sseChunk := "data: " + string(chunk) + "\n\n"
			w.Write([]byte(sseChunk))
			flusher.Flush()
		case <-done:
			w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			log.Printf("Stream done | %d tokens", wordCount)
			return
		case <-r.Context().Done():
			log.Printf("Client disconnected")
			return
		}
	}
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func shouldStream(body []byte) bool {
	var m map[string]interface{}
	if json.Unmarshal(body, &m) != nil {
		return false
	}
	if s, ok := m["stream"]; ok {
		if b, ok := s.(bool); ok {
			return b
		}
	}
	return false
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
