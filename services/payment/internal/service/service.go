package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/google/uuid"

	"logistics/pkg/events"
	"logistics/services/payment/internal/domain"
	"logistics/services/payment/internal/provider"
	"logistics/services/payment/internal/repository"
)

type CreateIntentInput struct {
	TrackingNumber string
	CustomerID     string
	AmountMinor    int64
	Currency       string
}

type CreateIntentResult struct {
	Payment      *domain.Payment
	ClientSecret string
}

type PaymentService interface {
	CreateIntent(ctx context.Context, in CreateIntentInput) (*CreateIntentResult, error)
	Get(ctx context.Context, id string) (*domain.Payment, error)
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

type paymentService struct {
	repo repository.PaymentRepository
	prov provider.PaymentProvider
	pub  events.Publisher
	log  *slog.Logger
}

func New(repo repository.PaymentRepository, prov provider.PaymentProvider, pub events.Publisher, log *slog.Logger) PaymentService {
	return &paymentService{repo: repo, prov: prov, pub: pub, log: log}
}

var currencyRe = regexp.MustCompile(`^[a-z]{3}$`)

func (s *paymentService) CreateIntent(ctx context.Context, in CreateIntentInput) (*CreateIntentResult, error) {
	if in.AmountMinor <= 0 {
		return nil, fmt.Errorf("%w: amount_minor must be > 0", domain.ErrValidation)
	}
	if !currencyRe.MatchString(in.Currency) {
		return nil, fmt.Errorf("%w: currency must be a lower-case ISO 4217 code", domain.ErrValidation)
	}
	p := &domain.Payment{
		ID: uuid.NewString(), TrackingNumber: in.TrackingNumber, CustomerID: in.CustomerID,
		AmountMinor: in.AmountMinor, Currency: in.Currency, Status: domain.StatusPending,
	}
	// Persist first: the webhook can only ever refer to a row that exists.
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	intent, err := s.prov.CreateIntent(ctx, provider.CreateIntentParams{
		PaymentID: p.ID, TrackingNumber: p.TrackingNumber, AmountMinor: p.AmountMinor, Currency: p.Currency,
	})
	if err != nil {
		if mErr := s.repo.MarkFailed(ctx, p.ID); mErr != nil {
			s.log.Error("mark failed", "payment", p.ID, "err", mErr)
		}
		return nil, fmt.Errorf("create provider intent: %w", err)
	}
	if err := s.repo.SetIntent(ctx, p.ID, intent.ID); err != nil {
		return nil, err
	}
	p.ProviderIntent = intent.ID
	return &CreateIntentResult{Payment: p, ClientSecret: intent.ClientSecret}, nil
}

func (s *paymentService) Get(ctx context.Context, id string) (*domain.Payment, error) {
	return s.repo.Get(ctx, id)
}

// HandleWebhook returns nil when the event is handled or deliberately ignored
// (so the provider stops retrying) and an error when it should be redelivered.
func (s *paymentService) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	ev, err := s.prov.ParseWebhook(payload, signature)
	if err != nil {
		return err
	}
	var to domain.Status
	var evType string
	switch ev.Type {
	case provider.EventPaymentSucceeded:
		to, evType = domain.StatusSucceeded, events.PaymentSucceeded
	case provider.EventPaymentFailed:
		to, evType = domain.StatusFailed, events.PaymentFailed
	default:
		return nil // event types we don't subscribe to
	}
	if ev.PaymentID == "" {
		s.log.Warn("webhook without payment_id metadata", "event", ev.ID)
		return nil
	}

	res, err := s.repo.ApplyWebhook(ctx, ev.ID, ev.PaymentID, to)
	if errors.Is(err, domain.ErrNotFound) {
		s.log.Warn("webhook for unknown payment", "event", ev.ID, "payment", ev.PaymentID)
		return nil // retrying will not help
	}
	if err != nil {
		return err // 5xx => provider redelivers
	}
	if !res.Changed && !res.Duplicate {
		return nil // out-of-order / not allowed transition: nothing to announce
	}

	// A duplicate delivery re-publishes too: the previous attempt may have
	// committed but failed to publish. The Kafka event id is the provider's
	// event id, so consumers can de-duplicate.
	out, err := events.New(evType, res.Payment.TrackingNumber, res.Payment)
	if err != nil {
		return err
	}
	out.ID = ev.ID
	if err := s.pub.Publish(ctx, events.TopicPayment, out); err != nil {
		return fmt.Errorf("publish: %w", err) // 5xx => redelivery retries the publish
	}
	return nil
}
