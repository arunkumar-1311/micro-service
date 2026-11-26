// Package provider isolates the payment gateway (Stripe) behind an interface.
package provider

import "context"

type CreateIntentParams struct {
	PaymentID      string // also used as the idempotency key
	TrackingNumber string
	AmountMinor    int64
	Currency       string
}

type Intent struct {
	ID           string
	ClientSecret string // returned to the browser/app to confirm the payment
}

// Webhook event kinds the service understands.
const (
	EventPaymentSucceeded = "payment_intent.succeeded"
	EventPaymentFailed    = "payment_intent.payment_failed"
)

type WebhookEvent struct {
	ID        string // provider event id, used for de-duplication
	Type      string
	IntentID  string
	PaymentID string // our id, round-tripped via provider metadata
}

type PaymentProvider interface {
	CreateIntent(ctx context.Context, p CreateIntentParams) (Intent, error)
	// ParseWebhook authenticates the payload (domain.ErrInvalidSignature on
	// failure) and decodes it.
	ParseWebhook(payload []byte, signatureHeader string) (*WebhookEvent, error)
}
