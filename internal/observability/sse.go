package observability

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func SSEHandler(bus *EventBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// CORS is intentionally not opened: the stream is authenticated and
		// served same-origin, so no Access-Control-Allow-Origin is sent.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		filterID := r.URL.Query().Get("request_id")
		subID, ch := bus.Subscribe(filterID)
		defer bus.Unsubscribe(subID)

		history := bus.History()
		for _, evt := range history {
			if filterID == "" || filterID == evt.RequestID {
				writeSSE(w, flusher, evt)
			}
		}

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return
				}
				writeSSE(w, flusher, evt)
			case <-heartbeat.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				log.Printf("SSE client disconnected")
				return
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
