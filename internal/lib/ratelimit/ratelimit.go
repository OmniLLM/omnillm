// Package ratelimit provides rate limiting functionality
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type reservation chan struct{}

type RateLimiter struct {
	mu              sync.Mutex
	intervalSeconds int
	waitOnLimit     bool
	lastGranted     time.Time
	queue           []reservation
	changed         chan struct{}
}

func NewRateLimiter(intervalSeconds int, waitOnLimit bool) *RateLimiter {
	return &RateLimiter{
		intervalSeconds: intervalSeconds,
		waitOnLimit:     waitOnLimit,
		changed:         make(chan struct{}),
	}
}

func (rl *RateLimiter) CheckAndWait(ctx context.Context) error {
	if rl.intervalSeconds <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	requiredInterval := time.Duration(rl.intervalSeconds) * time.Second
	rl.mu.Lock()
	now := time.Now()
	if len(rl.queue) == 0 && (rl.lastGranted.IsZero() || !now.Before(rl.lastGranted.Add(requiredInterval))) {
		rl.lastGranted = now
		rl.mu.Unlock()
		return nil
	}

	waitTime := rl.nextAvailabilityLocked(requiredInterval).Sub(now)
	if !rl.waitOnLimit {
		rl.mu.Unlock()
		log.Warn().
			Float64("wait_seconds", waitTime.Seconds()).
			Float64("required_seconds", requiredInterval.Seconds()).
			Msg("Rate limit exceeded")
		return fmt.Errorf("rate limit exceeded. Need to wait %v more", waitTime)
	}

	res := make(reservation)
	rl.queue = append(rl.queue, res)
	rl.notifyLocked()
	rl.mu.Unlock()

	log.Warn().
		Float64("wait_seconds", waitTime.Seconds()).
		Msg("Rate limit reached. Waiting before proceeding...")

	for {
		rl.mu.Lock()
		position := rl.reservationIndexLocked(res)
		if position < 0 {
			rl.mu.Unlock()
			return ctx.Err()
		}
		now = time.Now()
		readyAt := rl.lastGranted.Add(requiredInterval)
		if position == 0 && !now.Before(readyAt) {
			rl.queue = rl.queue[1:]
			rl.lastGranted = now
			rl.notifyLocked()
			rl.mu.Unlock()
			log.Info().Msg("Rate limit wait completed, proceeding with request")
			return nil
		}
		changed := rl.changed
		waitTime = readyAt.Sub(now)
		if waitTime < 0 {
			waitTime = 0
		}
		rl.mu.Unlock()

		timer := time.NewTimer(waitTime)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			rl.cancelReservation(res)
			return ctx.Err()
		case <-changed:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (rl *RateLimiter) nextAvailabilityLocked(interval time.Duration) time.Time {
	if rl.lastGranted.IsZero() {
		return time.Now()
	}
	return rl.lastGranted.Add(time.Duration(len(rl.queue)+1) * interval)
}

func (rl *RateLimiter) reservationIndexLocked(target reservation) int {
	for i, queued := range rl.queue {
		if queued == target {
			return i
		}
	}
	return -1
}

func (rl *RateLimiter) cancelReservation(target reservation) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	index := rl.reservationIndexLocked(target)
	if index < 0 {
		return
	}
	copy(rl.queue[index:], rl.queue[index+1:])
	rl.queue = rl.queue[:len(rl.queue)-1]
	rl.notifyLocked()
}

func (rl *RateLimiter) notifyLocked() {
	close(rl.changed)
	rl.changed = make(chan struct{})
}

func (rl *RateLimiter) GetIntervalSeconds() int {
	return rl.intervalSeconds
}

func (rl *RateLimiter) GetWaitOnLimit() bool {
	return rl.waitOnLimit
}
