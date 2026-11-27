package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"logistics/pkg/cache"
	"logistics/pkg/events"
	"logistics/services/notification/internal/sender"
)

// ErrPhoneUnknown is returned when we have no number for a tracking number
// yet (e.g. the shipment event hasn't been consumed). It is retried.
var ErrPhoneUnknown = errors.New("recipient phone unknown")

const (
	phoneTTL = 90 * 24 * time.Hour
	dedupTTL = 7 * 24 * time.Hour
)

type shipmentPayload struct {
	TrackingNumber string `json:"tracking_number"`
	RecipientName  string `json:"recipient_name"`
	RecipientPhone string `json:"recipient_phone"`
	Status         string `json:"status"`
}

type paymentPayload struct {
	TrackingNumber string `json:"tracking_number"`
	AmountMinor    int64  `json:"amount_minor"`
	Currency       string `json:"currency"`
}

// Service turns domain events into SMS messages.
type Service interface {
	Handle(ctx context.Context, e events.Event) error
}

type notifier struct {
	sms   sender.SMSSender
	cache cache.Cache
	log   *slog.Logger
}

func New(sms sender.SMSSender, c cache.Cache, log *slog.Logger) Service {
	return &notifier{sms: sms, cache: c, log: log}
}

func phoneKey(tracking string) string { return "notify:phone:" + tracking }

func (n *notifier) Handle(ctx context.Context, e events.Event) error {
	switch e.Type {
	case events.ShipmentCreated, events.ShipmentStatusChanged:
		var p shipmentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil // malformed payload will never succeed; don't retry
		}
		if p.RecipientPhone != "" {
			if err := n.cache.Set(ctx, phoneKey(p.TrackingNumber), []byte(p.RecipientPhone), phoneTTL); err != nil {
				return fmt.Errorf("remember phone: %w", err)
			}
		}
		body, ok := shipmentMessage(e.Type, p)
		if !ok {
			return nil
		}
		return n.sendOnce(ctx, e.ID, p.RecipientPhone, body)

	case events.PaymentSucceeded, events.PaymentFailed:
		var p paymentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil
		}
		b, err := n.cache.Get(ctx, phoneKey(p.TrackingNumber))
		if err != nil {
			if errors.Is(err, cache.ErrMiss) {
				return fmt.Errorf("%w: %s", ErrPhoneUnknown, p.TrackingNumber)
			}
			return err
		}
		body, _ := paymentMessage(e.Type, p)
		return n.sendOnce(ctx, e.ID, string(b), body)
	}
	return nil // not an event we notify on
}

// sendOnce makes delivery idempotent: Kafka is at-least-once, so the same
// event can arrive twice. The claim is released if sending fails so the
// retry can actually send.
func (n *notifier) sendOnce(ctx context.Context, eventID, to, body string) error {
	if to == "" {
		n.log.Warn("no phone number, skipping", "event", eventID)
		return nil
	}
	key := "notify:sent:" + eventID
	claimed, err := n.cache.SetNX(ctx, key, []byte("1"), dedupTTL)
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	if !claimed {
		n.log.Info("duplicate event, already notified", "event", eventID)
		return nil
	}
	if err := n.sms.Send(ctx, to, body); err != nil {
		if dErr := n.cache.Delete(ctx, key); dErr != nil {
			n.log.Error("release claim failed", "event", eventID, "err", dErr)
		}
		return fmt.Errorf("send sms to %s: %w", sender.Mask(to), err)
	}
	return nil
}
