package observability

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Stage string

const (
	StageRequestReceived  Stage = "request_received"
	StageBodyRead         Stage = "body_read"
	StageJSONValidated    Stage = "json_validated"
	StageLoadingAPIKey    Stage = "loading_api_key"
	StageBuildingRequest  Stage = "building_request"
	StageSettingHeaders   Stage = "setting_headers"
	StageSendingRequest   Stage = "sending_request"
	StageWaitingResponse  Stage = "waiting_response"
	StageReadingResponse  Stage = "reading_response"
	StageValidatingStatus Stage = "validating_status"
	StageResponseSent     Stage = "response_sent"
	StageError            Stage = "error"
	StageTokenStream      Stage = "token_stream"
	StageInfo             Stage = "info"
)

type Event struct {
	ID        string          `json:"id"`
	Stage     Stage           `json:"stage"`
	Status    string          `json:"status"`
	Timestamp int64           `json:"timestamp"`
	Duration  int64           `json:"duration"`
	Data      json.RawMessage `json:"data"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
}

type subscriber struct {
	ch       chan Event
	filterID string
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[int64]subscriber
	nextID      int64
	history     []Event
	maxHistory  int
}

var DefaultBus = NewEventBus(200)

func NewEventBus(maxHistory int) *EventBus {
	return &EventBus{
		subscribers: make(map[int64]subscriber),
		maxHistory:  maxHistory,
	}
}

func (b *EventBus) Subscribe(filterRequestID string) (int64, <-chan Event) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subscribers[id] = subscriber{ch: ch, filterID: filterRequestID}
	b.mu.Unlock()
	return id, ch
}

func (b *EventBus) Unsubscribe(id int64) {
	b.mu.Lock()
	if sub, ok := b.subscribers[id]; ok {
		close(sub.ch)
		delete(b.subscribers, id)
	}
	b.mu.Unlock()
}

func (b *EventBus) Publish(event Event) {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().UnixMilli()
	}
	if event.Status == "" {
		event.Status = "info"
	}

	b.mu.Lock()
	b.history = append(b.history, event)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	snapshot := make([]subscriber, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		if s.filterID == "" || s.filterID == event.RequestID {
			snapshot = append(snapshot, s)
		}
	}
	b.mu.Unlock()

	for _, s := range snapshot {
		select {
		case s.ch <- event:
		default:
		}
	}
}

func (b *EventBus) History() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	h := make([]Event, len(b.history))
	copy(h, b.history)
	return h
}

func NewEvent(requestID string, stage Stage, status string) Event {
	return Event{
		ID:        fmt.Sprintf("%s-%s-%d", requestID, stage, time.Now().UnixNano()),
		Stage:     stage,
		Status:    status,
		Timestamp: time.Now().UnixMilli(),
		RequestID: requestID,
	}
}

func NewDataEvent(requestID string, stage Stage, status string, data interface{}) Event {
	e := NewEvent(requestID, stage, status)
	if data != nil {
		raw, _ := json.Marshal(data)
		e.Data = raw
	}
	return e
}

func NewMessageEvent(requestID string, stage Stage, status string, msg string) Event {
	e := NewEvent(requestID, stage, status)
	e.Message = msg
	return e
}

func NewDurationEvent(requestID string, stage Stage, status string, duration time.Duration) Event {
	e := NewEvent(requestID, stage, status)
	e.Duration = duration.Milliseconds()
	return e
}
