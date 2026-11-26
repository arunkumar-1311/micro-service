package domain

import (
	"errors"
	"time"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

var (
	ErrNotFound   = errors.New("payment not found")
	ErrValidation = errors.New("validation failed")
	// ErrInvalidSignature is returned when a webhook fails authentication.
	ErrInvalidSignature = errors.New("invalid webhook signature")
)

// CanMoveTo: a failed intent can be retried by the customer and later
// succeed; a succeeded payment is final.
func (s Status) CanMoveTo(next Status) bool {
	switch s {
	case StatusPending:
		return next == StatusSucceeded || next == StatusFailed
	case StatusFailed:
		return next == StatusSucceeded
	}
	return false
}

type Payment struct {
	ID             string    `json:"id"`
	TrackingNumber string    `json:"tracking_number"`
	CustomerID     string    `json:"customer_id"`
	AmountMinor    int64     `json:"amount_minor"` // smallest currency unit (cents)
	Currency       string    `json:"currency"`     // lower-case ISO 4217
	Status         Status    `json:"status"`
	ProviderIntent string    `json:"provider_intent_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
