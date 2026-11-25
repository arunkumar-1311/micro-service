// Package events defines the Kafka event envelope, topics and the
// Publisher/Consumer abstractions used for async service communication.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TopicShipment     = "shipment.events"
	TopicPayment      = "payment.events"
	TopicNotification = "notification.commands"
)

const (
	ShipmentCreated       = "shipment.created"
	ShipmentStatusChanged = "shipment.status_changed"
	PaymentSucceeded      = "payment.succeeded"
	PaymentFailed         = "payment.failed"
)

// Event is the envelope for every message on the bus.
type Event struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Key        string          `json:"key"` // partition key, e.g. tracking number
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func New(eventType, key string, payload any) (Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{ID: uuid.NewString(), Type: eventType, Key: key, OccurredAt: time.Now().UTC(), Payload: b}, nil
}

type Publisher interface {
	Publish(ctx context.Context, topic string, e Event) error
	Close() error
}

type Handler func(ctx context.Context, e Event) error

type Consumer interface {
	// Run blocks, delivering events to h until ctx is cancelled.
	Run(ctx context.Context, h Handler) error
	Close() error
}
