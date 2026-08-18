package observability

import "testing"

func TestMakeCaptureDecisionDisabled(t *testing.T) {
	d := MakeCaptureDecision(CaptureModeDisabled, []byte("payload"), 1.0)
	if d.ShouldCapture {
		t.Fatalf("disabled mode should not capture")
	}
}

func TestMakeCaptureDecisionFull(t *testing.T) {
	d := MakeCaptureDecision(CaptureModeFull, []byte("payload"), 1.0)
	if !d.ShouldCapture || !d.StorePayload || !d.ComputeHash {
		t.Fatalf("full mode should capture and store: %+v", d)
	}
	if d.FinalMode != CaptureModeFull {
		t.Fatalf("final mode = %q, want full", d.FinalMode)
	}
}

func TestMakeCaptureDecisionHashOnly(t *testing.T) {
	d := MakeCaptureDecision(CaptureModeHashOnly, []byte("payload"), 1.0)
	if !d.ShouldCapture || d.StorePayload || !d.ComputeHash {
		t.Fatalf("hash_only mode should capture hash but not store blob: %+v", d)
	}
	if d.FinalMode != CaptureModeHashOnly {
		t.Fatalf("final mode = %q, want hash_only", d.FinalMode)
	}
}

func TestMakeCaptureDecisionSampled(t *testing.T) {
	// rate 0.0 -> never sampled
	if d := MakeCaptureDecision(CaptureModeSampled, []byte("payload"), 0.0); d.ShouldCapture {
		t.Fatalf("sample rate 0.0 should never capture")
	}
	// rate 1.0 -> always sampled
	if d := MakeCaptureDecision(CaptureModeSampled, []byte("payload"), 1.0); !d.ShouldCapture {
		t.Fatalf("sample rate 1.0 should always capture")
	}
	// deterministic: same payload + rate always yields same decision
	a := MakeCaptureDecision(CaptureModeSampled, []byte("deterministic"), 0.5)
	b := MakeCaptureDecision(CaptureModeSampled, []byte("deterministic"), 0.5)
	if a.ShouldCapture != b.ShouldCapture {
		t.Fatalf("sampling not deterministic for identical input: %v vs %v", a, b)
	}
}
