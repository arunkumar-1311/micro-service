package repository

import (
	"context"

	"logistics/services/payment/internal/domain"
)

type WebhookResult struct {
	Payment   *domain.Payment
	Duplicate bool // event id already processed
	Changed   bool // status actually moved
}

type PaymentRepository interface {
	Create(ctx context.Context, p *domain.Payment) error
	Get(ctx context.Context, id string) (*domain.Payment, error)
	SetIntent(ctx context.Context, id, intentID string) error
	MarkFailed(ctx context.Context, id string) error
	// ApplyWebhook records eventID and applies the status change in ONE
	// transaction, so a crash can never leave an event marked as processed
	// without its effect (and provider retries stay safe).
	ApplyWebhook(ctx context.Context, eventID, paymentID string, to domain.Status) (*WebhookResult, error)
}
