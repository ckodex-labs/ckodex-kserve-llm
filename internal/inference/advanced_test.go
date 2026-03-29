package inference

import (
	"errors"
	"testing"
	"time"
)

func TestPriorityQueue_LoadShedding(t *testing.T) {
	// Queue with max depth 3
	pq := NewPriorityQueue(3)

	res := make(chan PriorityResult, 1)

	// Fill queue with 3 background requests
	for i := 0; i < 3; i++ {
		ok := pq.Enqueue(PriorityRequest{
			Priority: PriorityBackground,
			Result:   res,
		})
		if !ok {
			t.Fatalf("expected enqueue to succeed")
		}
	}

	if pq.Depth() != 3 {
		t.Fatalf("expected depth 3, got %d", pq.Depth())
	}

	// Enqueue an interactive request. This should shed one background request.
	interactiveRes := make(chan PriorityResult, 1)
	ok := pq.Enqueue(PriorityRequest{
		Priority: PriorityInteractive,
		Result:   interactiveRes,
	})
	if !ok {
		t.Fatalf("expected interactive request to succeed and shed a background one")
	}

	// Depth should remain 3
	if pq.Depth() != 3 {
		t.Fatalf("expected depth to stay at 3, got %d", pq.Depth())
	}

	// Check that a background request was rejected with ErrLoadShed
	shedResult := <-res
	if shedResult.Error == nil || !errors.Is(shedResult.Error, ErrLoadShed) {
		t.Fatalf("expected ErrLoadShed, got %v", shedResult.Error)
	}

	// Dequeue should yield the interactive request first
	req, ok := pq.Dequeue()
	if !ok || req.Priority != PriorityInteractive {
		t.Fatalf("expected interactive request to be dequeued first")
	}
}

func TestPriorityQueue_DeadlineExceeded(t *testing.T) {
	pq := NewPriorityQueue(10)
	res := make(chan PriorityResult, 1)

	// Enqueue an already expired request
	pq.Enqueue(PriorityRequest{
		Priority: PriorityInteractive,
		Deadline: time.Now().Add(-1 * time.Second),
		Result:   res,
	})

	// Dequeue should drop and return the next valid request (none in this case)
	req, ok := pq.Dequeue()
	if ok {
		t.Fatalf("expected no valid request, got %v", req.Priority)
	}

	// Result channel should have ErrDeadlineExceeded
	expiredResult := <-res
	if expiredResult.Error == nil || !errors.Is(expiredResult.Error, ErrDeadlineExceeded) {
		t.Fatalf("expected ErrDeadlineExceeded, got %v", expiredResult.Error)
	}
}
