package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"logistics/pkg/resilience"
	"logistics/services/payment/internal/domain"
)

const webhookTolerance = 5 * time.Minute

type Stripe struct {
	secretKey     string
	webhookSecret string
	baseURL       string
	http          *http.Client
	cb            *resilience.CircuitBreaker
	now           func() time.Time
}

func NewStripe(secretKey, webhookSecret string) *Stripe {
	return &Stripe{
		secretKey: secretKey, webhookSecret: webhookSecret,
		baseURL: "https://api.stripe.com",
		http:    &http.Client{Timeout: 15 * time.Second},
		cb:      resilience.NewCircuitBreaker(5, 30*time.Second),
		now:     time.Now,
	}
}

type apiError struct {
	status int
	body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("stripe api %d: %s", e.status, e.body) }

// retryable: network errors, 429 and 5xx. 4xx (bad request, card errors) are not.
func retryable(err error) bool {
	if ae, ok := err.(*apiError); ok {
		return ae.status == http.StatusTooManyRequests || ae.status >= 500
	}
	return true
}

func (s *Stripe) CreateIntent(ctx context.Context, p CreateIntentParams) (Intent, error) {
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(p.AmountMinor, 10))
	form.Set("currency", p.Currency)
	form.Set("automatic_payment_methods[enabled]", "true")
	form.Set("metadata[payment_id]", p.PaymentID)
	form.Set("metadata[tracking_number]", p.TrackingNumber)

	var out Intent
	// Breaker wraps the retries: a Stripe outage trips it and later calls fail fast.
	err := s.cb.Execute(func() error {
		return resilience.Retry(ctx, resilience.RetryConfig{
			MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second, Retryable: retryable,
		}, func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/payment_intents", strings.NewReader(form.Encode()))
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+s.secretKey)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			// Same key on every retry => Stripe never creates two intents.
			req.Header.Set("Idempotency-Key", "pi-"+p.PaymentID)
			resp, err := s.http.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode >= 300 {
				return &apiError{status: resp.StatusCode, body: string(body)}
			}
			var r struct {
				ID           string `json:"id"`
				ClientSecret string `json:"client_secret"`
			}
			if err := json.Unmarshal(body, &r); err != nil {
				return err
			}
			out = Intent{ID: r.ID, ClientSecret: r.ClientSecret}
			return nil
		})
	})
	return out, err
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

func (s *Stripe) ParseWebhook(payload []byte, sigHeader string) (*WebhookEvent, error) {
	if err := s.verifySignature(payload, sigHeader); err != nil {
		return nil, err
	}
	var ev stripeEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, fmt.Errorf("decode webhook: %w", err)
	}
	return &WebhookEvent{
		ID: ev.ID, Type: ev.Type,
		IntentID: ev.Data.Object.ID, PaymentID: ev.Data.Object.Metadata["payment_id"],
	}, nil
}

// verifySignature implements Stripe's scheme: header "t=<unix>,v1=<hex>[,v1=...]",
// signed payload "<t>.<body>", HMAC-SHA256 with the endpoint secret.
func (s *Stripe) verifySignature(payload []byte, header string) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return domain.ErrInvalidSignature
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return domain.ErrInvalidSignature
	}
	if d := s.now().Sub(time.Unix(unix, 0)); d > webhookTolerance || d < -webhookTolerance {
		return domain.ErrInvalidSignature // replay protection
	}
	mac := hmac.New(sha256.New, []byte(s.webhookSecret))
	mac.Write([]byte(ts + "."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, sig := range sigs {
		if got, err := hex.DecodeString(sig); err == nil && hmac.Equal(got, expected) {
			return nil
		}
	}
	return domain.ErrInvalidSignature
}
