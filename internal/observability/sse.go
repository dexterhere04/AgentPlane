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
