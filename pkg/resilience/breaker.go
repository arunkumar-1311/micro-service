package resilience

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type state int

const (
	closed state = iota
	open
	halfOpen
)

// CircuitBreaker trips after N consecutive failures, rejects calls for
// the cooldown period, then lets a single probe through (half-open).
type CircuitBreaker struct {
	mu        sync.Mutex
	st        state
	failures  int
	openedAt  time.Time
	threshold int
	cooldown  time.Duration
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (b *CircuitBreaker) Execute(fn func() error) error {
	if err := b.before(); err != nil {
		return err
	}
	err := fn()
	b.after(err)
	return err
}

func (b *CircuitBreaker) before() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.st {
	case open:
		if time.Since(b.openedAt) < b.cooldown {
			return ErrCircuitOpen
		}
		b.st = halfOpen // allow one probe
	case halfOpen:
		return ErrCircuitOpen // probe already in flight
	}
	return nil
}

func (b *CircuitBreaker) after(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.st, b.failures = closed, 0
		return
	}
	b.failures++
	if b.st == halfOpen || b.failures >= b.threshold {
		b.st, b.openedAt = open, time.Now()
	}
}
