package observability

import (
    "testing"
    "time"
)

func TestNoopStoreHelpers(t *testing.T) {
    // Ensure default nil store is safe to call via helpers
    SetStore(NewNoopStore())
    defer SetStore(nil)

    _, err := CapturePromptPayload(Payload{RequestID: "r1", Timestamp: time.Now(), Payload: []byte("x")})
    if err != nil {
        t.Fatalf("CapturePromptPayload returned error: %v", err)
    }
    _, err = CaptureResponsePayload(Payload{RequestID: "r1", Timestamp: time.Now(), Payload: []byte("y")})
    if err != nil {
        t.Fatalf("CaptureResponsePayload returned error: %v", err)
    }
    if err := CaptureToolCall(ToolCall{ID: "t1", Timestamp: time.Now()}); err != nil {
        t.Fatalf("CaptureToolCall error: %v", err)
    }
}
