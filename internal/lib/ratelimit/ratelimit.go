// Package ratelimit provides rate limiting functionality
package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type RateLimiter struct {
	mu              sync.Mutex
	intervalSeconds int
	waitOnLimit     bool
	nextAllowedTime time.Time
}

func NewRateLimiter(intervalSeconds int, waitOnLimit bool) *RateLimiter {
	return &RateLimiter{
		intervalSeconds: intervalSeconds,
		waitOnLimit:     waitOnLimit,
	}
}

func (rl *RateLimiter) CheckAndWait() error {
	if rl.intervalSeconds <= 0 {
		return nil
	}

	requiredInterval := time.Duration(rl.intervalSeconds) * time.Second

	rl.mu.Lock()
	now := time.Now()

	if rl.nextAllowedTime.IsZero() || !now.Before(rl.nextAllowedTime) {
		rl.nextAllowedTime = now.Add(requiredInterval)
		rl.mu.Unlock()
		return nil
	}

	waitTime := rl.nextAllowedTime.Sub(now)
	if !rl.waitOnLimit {
		rl.mu.Unlock()
		log.Warn().
			Float64("wait_seconds", waitTime.Seconds()).
			Float64("required_seconds", requiredInterval.Seconds()).
			Msg("Rate limit exceeded")
		return fmt.Errorf("rate limit exceeded. Need to wait %v more", waitTime)
	}

	rl.nextAllowedTime = rl.nextAllowedTime.Add(requiredInterval)
	rl.mu.Unlock()

	log.Warn().
		Float64("wait_seconds", waitTime.Seconds()).
		Msg("Rate limit reached. Waiting before proceeding...")

	time.Sleep(waitTime)

	log.Info().Msg("Rate limit wait completed, proceeding with request")
	return nil
}

func (rl *RateLimiter) GetIntervalSeconds() int {
	return rl.intervalSeconds
}

func (rl *RateLimiter) GetWaitOnLimit() bool {
	return rl.waitOnLimit
}
