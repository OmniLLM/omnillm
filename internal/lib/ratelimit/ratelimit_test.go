package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(0, false)
	if rl.GetIntervalSeconds() != 0 {
		t.Errorf("expected interval=0, got %d", rl.GetIntervalSeconds())
	}
	if rl.GetWaitOnLimit() {
		t.Error("expected waitOnLimit=false")
	}
}

func TestNewRateLimiter_WithValues(t *testing.T) {
	rl := NewRateLimiter(5, true)
	if rl.GetIntervalSeconds() != 5 {
		t.Errorf("expected interval=5, got %d", rl.GetIntervalSeconds())
	}
	if !rl.GetWaitOnLimit() {
		t.Error("expected waitOnLimit=true")
	}
}

func TestCheckAndWait_ZeroInterval(t *testing.T) {
	rl := NewRateLimiter(0, false)
	// Should never block or error
	for i := 0; i < 10; i++ {
		if err := rl.CheckAndWait(context.Background()); err != nil {
			t.Fatalf("unexpected error with zero interval: %v", err)
		}
	}
}

func TestCheckAndWait_FirstCallAlwaysSucceeds(t *testing.T) {
	rl := NewRateLimiter(60, false) // 60 second interval, no wait
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should always succeed, got error: %v", err)
	}
}

func TestCheckAndWait_SecondCallFailsImmediately(t *testing.T) {
	rl := NewRateLimiter(60, false) // 60 second interval, fail immediately
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	// Second call immediately - should fail since 60s have not passed
	err := rl.CheckAndWait(context.Background())
	if err == nil {
		t.Error("second immediate call should fail with rate limit error")
	}
}

func TestCheckAndWait_WaitsWhenRequired(t *testing.T) {
	// Use 1-second interval with wait mode
	rl := NewRateLimiter(1, true)
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	// Second call should wait ~1s then succeed
	start := time.Now()
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("second call with waitOnLimit=true should not error: %v", err)
	}
	elapsed := time.Since(start)
	// Should have waited at least some fraction of the interval
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected to wait at least 100ms, waited %v", elapsed)
	}
}

func TestCheckAndWait_CancellationRemovesReservation(t *testing.T) {
	rl := NewRateLimiter(1, true)
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- rl.CheckAndWait(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("canceled waiter did not return promptly")
	}

	start := time.Now()
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("later waiter should succeed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("canceled reservation delayed later waiter: %v", elapsed)
	}
}

func TestCheckAndWait_AllowsCallAfterInterval(t *testing.T) {
	rl := NewRateLimiter(1, false) // 1-second interval, no wait
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	// Wait for the interval to pass
	time.Sleep(1100 * time.Millisecond)
	// Now it should succeed again
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("call after interval should succeed: %v", err)
	}
}

func TestCheckAndWait_ConcurrentWaitersSerialize(t *testing.T) {
	rl := NewRateLimiter(1, true)
	if err := rl.CheckAndWait(context.Background()); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}

	start := time.Now()
	results := make(chan time.Duration, 2)

	for range 2 {
		go func() {
			if err := rl.CheckAndWait(context.Background()); err != nil {
				t.Errorf("concurrent waiter should not error: %v", err)
				results <- 0
				return
			}
			results <- time.Since(start)
		}()
	}

	first := <-results
	second := <-results
	if first > second {
		first, second = second, first
	}

	if first < 900*time.Millisecond {
		t.Fatalf("expected first waiter to wait about 1s, got %v", first)
	}
	if second < 1900*time.Millisecond {
		t.Fatalf("expected second waiter to wait about 2s, got %v", second)
	}
}
