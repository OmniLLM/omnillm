package database

import (
	"sync"

	"github.com/rs/zerolog/log"
)

// asyncWorkers holds the bounded channel-based worker pools for fire-and-forget
// database writes that must never block or slow down the request path.
var asyncWorkers struct {
	lifecycle sync.Mutex
	mu        sync.Mutex
	metering  chan MeteringRecord
	lastUsed  chan string
	wg        sync.WaitGroup
	running   bool
}

const (
	meteringBufSize = 4096
	lastUsedBufSize = 4096
	meteringWorkers = 4
	lastUsedWorkers = 2
)

// StartAsyncWorkers starts the bounded background worker pools.
// If workers are already running they are stopped first (supports re-init in tests).
func StartAsyncWorkers() {
	asyncWorkers.lifecycle.Lock()
	defer asyncWorkers.lifecycle.Unlock()
	stopAsyncWorkers()

	asyncWorkers.mu.Lock()
	defer asyncWorkers.mu.Unlock()
	asyncWorkers.metering = make(chan MeteringRecord, meteringBufSize)
	asyncWorkers.lastUsed = make(chan string, lastUsedBufSize)
	asyncWorkers.running = true

	metering := asyncWorkers.metering
	lastUsed := asyncWorkers.lastUsed
	for range meteringWorkers {
		asyncWorkers.wg.Add(1)
		go func() {
			defer asyncWorkers.wg.Done()
			db := GetDatabase()
			for rec := range metering {
				if err := db.InsertMeteringRecord(rec); err != nil {
					log.Error().Err(err).Str("request_id", rec.RequestID).Msg("Failed to record metering data")
				}
			}
		}()
	}

	for range lastUsedWorkers {
		asyncWorkers.wg.Add(1)
		go func() {
			defer asyncWorkers.wg.Done()
			db := GetDatabase()
			for id := range lastUsed {
				if _, err := db.db.Exec(`UPDATE access_tokens SET last_used_at = datetime('now') WHERE id = ?`, id); err != nil {
					log.Debug().Err(err).Str("token_id", id).Msg("Failed to stamp access token last_used_at")
				}
			}
		}()
	}
}

// StopAsyncWorkers closes both worker channels and waits for all pending writes
// to finish. Channels are detached while holding the lifecycle mutex so no
// enqueuer can select a channel after it has been closed.
func StopAsyncWorkers() {
	asyncWorkers.lifecycle.Lock()
	defer asyncWorkers.lifecycle.Unlock()
	stopAsyncWorkers()
}

func stopAsyncWorkers() {
	asyncWorkers.mu.Lock()
	if !asyncWorkers.running {
		asyncWorkers.mu.Unlock()
		return
	}
	asyncWorkers.running = false
	metering := asyncWorkers.metering
	lastUsed := asyncWorkers.lastUsed
	asyncWorkers.metering = nil
	asyncWorkers.lastUsed = nil
	close(metering)
	close(lastUsed)
	asyncWorkers.mu.Unlock()

	asyncWorkers.wg.Wait()
}

// EnqueueMeteringRecord sends rec to the metering worker pool.
// If the buffer is full (overload), the record is dropped and a warning is logged.
func EnqueueMeteringRecord(rec MeteringRecord) {
	asyncWorkers.mu.Lock()
	defer asyncWorkers.mu.Unlock()
	if asyncWorkers.metering == nil {
		return
	}
	select {
	case asyncWorkers.metering <- rec:
	default:
		log.Warn().Str("request_id", rec.RequestID).Msg("Metering worker pool full, dropping record")
	}
}

// EnqueueLastUsedAt sends an access token ID to the last-used-at worker pool.
// If the buffer is full the update is silently skipped — this is metadata-only.
func EnqueueLastUsedAt(tokenID string) {
	asyncWorkers.mu.Lock()
	defer asyncWorkers.mu.Unlock()
	if asyncWorkers.lastUsed == nil {
		return
	}
	select {
	case asyncWorkers.lastUsed <- tokenID:
	default:
	}
}
