package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryEventuallySucceeds(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(context.Context) error {
		if calls++; calls < 3 {
			return errors.New("boom")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBreakerOpensAndRecovers(t *testing.T) {
	b := NewCircuitBreaker(2, 20*time.Millisecond)
	fail := func() error { return errors.New("x") }
	_ = b.Execute(fail)
	_ = b.Execute(fail)
	if err := b.Execute(func() error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want open, got %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := b.Execute(func() error { return nil }); err != nil {
		t.Fatalf("probe should pass: %v", err)
	}
}
