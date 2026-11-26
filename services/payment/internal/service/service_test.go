package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"logistics/pkg/events"
	"logistics/services/payment/internal/domain"
	"logistics/services/payment/internal/provider"
	"logistics/services/payment/internal/repository"
)

type fakeRepo struct {
	repository.PaymentRepository
	seen map[string]bool
	p    *domain.Payment
}

func (f *fakeRepo) ApplyWebhook(_ context.Context, eventID, paymentID string, to domain.Status) (*repository.WebhookResult, error) {
	if f.p == nil || f.p.ID != paymentID {
		return nil, domain.ErrNotFound
	}
	if f.seen[eventID] {
		return &repository.WebhookResult{Payment: f.p, Duplicate: true}, nil
	}
	f.seen[eventID] = true
	res := &repository.WebhookResult{Payment: f.p}
	if f.p.Status.CanMoveTo(to) {
		f.p.Status, res.Changed = to, true
	}
	return res, nil
}

type fakeProv struct {
	provider.PaymentProvider
	ev  *provider.WebhookEvent
	err error
}

func (f *fakeProv) ParseWebhook([]byte, string) (*provider.WebhookEvent, error) { return f.ev, f.err }

type fakePub struct{ got []events.Event }

func (f *fakePub) Publish(_ context.Context, _ string, e events.Event) error {
	f.got = append(f.got, e)
	return nil
}
func (f *fakePub) Close() error { return nil }

func newSvc(prov *fakeProv) (PaymentService, *fakePub, *fakeRepo) {
	repo := &fakeRepo{seen: map[string]bool{}, p: &domain.Payment{ID: "p1", TrackingNumber: "LG1", Status: domain.StatusPending}}
	pub := &fakePub{}
	return New(repo, prov, pub, slog.New(slog.NewTextHandler(io.Discard, nil))), pub, repo
}

func TestWebhookSucceededThenDuplicate(t *testing.T) {
	prov := &fakeProv{ev: &provider.WebhookEvent{ID: "evt1", Type: provider.EventPaymentSucceeded, PaymentID: "p1"}}
	svc, pub, repo := newSvc(prov)

	for i := 0; i < 2; i++ { // second call = Stripe redelivery
		if err := svc.HandleWebhook(context.Background(), nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	if repo.p.Status != domain.StatusSucceeded {
		t.Fatalf("status=%s", repo.p.Status)
	}
	if len(pub.got) != 2 || pub.got[0].ID != "evt1" || pub.got[1].ID != "evt1" || pub.got[0].Type != events.PaymentSucceeded {
		t.Fatalf("published=%+v", pub.got)
	}
}

func TestWebhookLateFailureDoesNotRegress(t *testing.T) {
	prov := &fakeProv{ev: &provider.WebhookEvent{ID: "evt2", Type: provider.EventPaymentFailed, PaymentID: "p1"}}
	svc, pub, repo := newSvc(prov)
	repo.p.Status = domain.StatusSucceeded

	if err := svc.HandleWebhook(context.Background(), nil, ""); err != nil {
		t.Fatal(err)
	}
	if repo.p.Status != domain.StatusSucceeded || len(pub.got) != 0 {
		t.Fatalf("status=%s published=%d", repo.p.Status, len(pub.got))
	}
}

func TestWebhookRejectsBadSignatureAndIgnoresOtherTypes(t *testing.T) {
	svc, _, _ := newSvc(&fakeProv{err: domain.ErrInvalidSignature})
	if err := svc.HandleWebhook(context.Background(), nil, ""); !errors.Is(err, domain.ErrInvalidSignature) {
		t.Fatalf("got %v", err)
	}
	svc, pub, _ := newSvc(&fakeProv{ev: &provider.WebhookEvent{ID: "e", Type: "charge.refunded"}})
	if err := svc.HandleWebhook(context.Background(), nil, ""); err != nil || len(pub.got) != 0 {
		t.Fatalf("err=%v pub=%d", err, len(pub.got))
	}
}
