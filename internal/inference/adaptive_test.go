package inference

import (
	"testing"
	"time"
)

func TestAdaptiveTimeout(t *testing.T) {
	min := 50 * time.Millisecond
	max := 500 * time.Millisecond
	adaptive := NewAdaptiveTimeout(1000, min, max)

	// Test boundary conditions initially
	if timeout := adaptive.Timeout(); timeout != max {
		t.Errorf("expected initial timeout to be max %v, got %v", max, timeout)
	}

	// Insert 100 samples of 100ms
	for i := 0; i < 100; i++ {
		adaptive.Record(100 * time.Millisecond)
	}

	// P99 should be 100ms. Timeout = 100ms * 1.2 = 120ms
	timeout := adaptive.Timeout()
	if timeout != 120*time.Millisecond {
		t.Errorf("expected 120ms timeout, got %v", timeout)
	}

	// Insert 1 outlier of 400ms (won't affect P50, but might affect P99 depending on sample size)
	// Since 1 out of 101 is ~1%, P99 might jump to 400ms
	adaptive.Record(400 * time.Millisecond)
	// Force recompute by reaching next 100 boundary
	for i := 0; i < 99; i++ {
		adaptive.Record(100 * time.Millisecond)
	}

	if p50 := adaptive.P50(); p50 != 100*time.Millisecond {
		t.Errorf("expected p50 to be 100ms, got %v", p50)
	}
}

func TestAdaptiveTimeout_Clamping(t *testing.T) {
	adaptive := NewAdaptiveTimeout(100, 100*time.Millisecond, 200*time.Millisecond)

	// Record values well below min (10ms)
	for i := 0; i < 100; i++ {
		adaptive.Record(10 * time.Millisecond)
	}
	if timeout := adaptive.Timeout(); timeout != 100*time.Millisecond {
		t.Errorf("expected clamped min timeout 100ms, got %v", timeout)
	}

	// Record values well above max (1000ms)
	for i := 0; i < 100; i++ {
		adaptive.Record(1000 * time.Millisecond)
	}
	if timeout := adaptive.Timeout(); timeout != 200*time.Millisecond {
		t.Errorf("expected clamped max timeout 200ms, got %v", timeout)
	}
}
