package database

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestAsyncWorkersConcurrentStartsRemainStoppable(t *testing.T) {
	var starters sync.WaitGroup
	for range 8 {
		starters.Add(1)
		go func() {
			defer starters.Done()
			StartAsyncWorkers()
		}()
	}
	starters.Wait()

	done := make(chan struct{})
	go func() {
		StopAsyncWorkers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown deadlocked after concurrent starts")
	}
	StartAsyncWorkers()
}

func TestAsyncWorkersConcurrentEnqueueAndRestart(t *testing.T) {
	StartAsyncWorkers()
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 250 {
				EnqueueMeteringRecord(MeteringRecord{RequestID: fmt.Sprintf("race-%d-%d", worker, i)})
				EnqueueLastUsedAt("missing-token")
			}
		}()
	}

	for range 10 {
		StopAsyncWorkers()
		StartAsyncWorkers()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		StopAsyncWorkers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent enqueue and restart deadlocked")
	}
	StartAsyncWorkers()
}
