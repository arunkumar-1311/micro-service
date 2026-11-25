// Package resilience provides retry with backoff and a circuit breaker.
package resilience

import (
	"context"
	"math/rand"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Retryable decides if an error is worth retrying. nil => always retry.
	Retryable func(error) bool
}

var DefaultRetry = RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: 2 * time.Second}

// Retry runs fn with exponential backoff + jitter until success, a
// non-retryable error, ctx cancellation, or attempts are exhausted.
func Retry(ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) error) error {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	var err error
	delay := cfg.BaseDelay
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err = fn(ctx); err == nil {
			return nil
		}
		if cfg.Retryable != nil && !cfg.Retryable(err) {
			return err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		sleep := delay
		if sleep > 0 {
			sleep = sleep/2 + time.Duration(rand.Int63n(int64(sleep/2)+1))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
		if delay *= 2; cfg.MaxDelay > 0 && delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
	return err
}
