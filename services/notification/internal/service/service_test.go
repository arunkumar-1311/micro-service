package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"logistics/pkg/cache"
	"logistics/pkg/events"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(_ context.Context, k string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.m[k]; ok {
		return v, nil
	}
	return nil, cache.ErrMiss
}
func (c *memCache) Set(_ context.Context, k string, v []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
	return nil
}
func (c *memCache) Delete(_ context.Context, ks ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range ks {
		delete(c.m, k)
	}
	return nil
}
func (c *memCache) SetNX(_ context.Context, k string, v []byte, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[k]; ok {
		return false, nil
	}
	c.m[k] = v
	return true, nil
}

type fakeSMS struct {
	sent []string // "to|body"
	fail bool
}

func (f *fakeSMS) Send(_ context.Context, to, body string) error {
	if f.fail {
		return errors.New("boom")
	}
	f.sent = append(f.sent, to+"|"+body)
	return nil
}

func setup() (Service, *fakeSMS) {
	sms := &fakeSMS{}
	return New(sms, &memCache{m: map[string][]byte{}}, slog.New(slog.NewTextHandler(io.Discard, nil))), sms
}

func ev(t *testing.T, id, typ string, payload any) events.Event {
	e, err := events.New(typ, "k", payload)
	if err != nil {
		t.Fatal(err)
	}
	e.ID = id
	return e
}

func TestShipmentCreatedSendsOnceEvenIfRedelivered(t *testing.T) {
	svc, sms := setup()
	e := ev(t, "e1", events.ShipmentCreated, shipmentPayload{TrackingNumber: "LG1", RecipientName: "Ann", RecipientPhone: "+15551234567"})
	for i := 0; i < 2; i++ {
		if err := svc.Handle(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if len(sms.sent) != 1 {
		t.Fatalf("sent=%v", sms.sent)
	}
}

func TestInTransitIsSilentButPaymentUsesRememberedPhone(t *testing.T) {
	svc, sms := setup()
	ctx := context.Background()
	_ = svc.Handle(ctx, ev(t, "e1", events.ShipmentStatusChanged, shipmentPayload{TrackingNumber: "LG1", RecipientPhone: "+15551234567", Status: "IN_TRANSIT"}))
	if len(sms.sent) != 0 {
		t.Fatalf("IN_TRANSIT must not SMS: %v", sms.sent)
	}
	if err := svc.Handle(ctx, ev(t, "e2", events.PaymentSucceeded, paymentPayload{TrackingNumber: "LG1", AmountMinor: 1250, Currency: "usd"})); err != nil {
		t.Fatal(err)
	}
	if len(sms.sent) != 1 || sms.sent[0] != "+15551234567|Payment of 12.50 USD received for shipment LG1." {
		t.Fatalf("sent=%v", sms.sent)
	}
}

func TestPaymentWithUnknownPhoneIsRetryable(t *testing.T) {
	svc, _ := setup()
	err := svc.Handle(context.Background(), ev(t, "e1", events.PaymentFailed, paymentPayload{TrackingNumber: "NOPE"}))
	if !errors.Is(err, ErrPhoneUnknown) {
		t.Fatalf("got %v", err)
	}
}

func TestFailedSendReleasesClaimSoRetrySends(t *testing.T) {
	svc, sms := setup()
	e := ev(t, "e1", events.ShipmentCreated, shipmentPayload{TrackingNumber: "LG1", RecipientPhone: "+15551234567"})
	sms.fail = true
	if err := svc.Handle(context.Background(), e); err == nil {
		t.Fatal("want error")
	}
	sms.fail = false
	if err := svc.Handle(context.Background(), e); err != nil || len(sms.sent) != 1 {
		t.Fatalf("err=%v sent=%v", err, sms.sent)
	}
}
