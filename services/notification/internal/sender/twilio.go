package sender

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"logistics/pkg/resilience"
)

type Twilio struct {
	accountSID, authToken, from string
	baseURL                     string
	http                        *http.Client
	cb                          *resilience.CircuitBreaker
}

func NewTwilio(accountSID, authToken, from string) *Twilio {
	return &Twilio{
		accountSID: accountSID, authToken: authToken, from: from,
		baseURL: "https://api.twilio.com",
		http:    &http.Client{Timeout: 10 * time.Second},
		cb:      resilience.NewCircuitBreaker(5, 30*time.Second),
	}
}

type apiError struct {
	status int
	body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("twilio api %d: %s", e.status, e.body) }

// Network errors, 429 and 5xx are transient; other 4xx (bad number, auth) are not.
func retryable(err error) bool {
	if ae, ok := err.(*apiError); ok {
		return ae.status == http.StatusTooManyRequests || ae.status >= 500
	}
	return true
}

func (t *Twilio) Send(ctx context.Context, to, body string) error {
	form := url.Values{"To": {to}, "From": {t.from}, "Body": {body}}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", t.baseURL, t.accountSID)

	return t.cb.Execute(func() error {
		return resilience.Retry(ctx, resilience.RetryConfig{
			MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second, Retryable: retryable,
		}, func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
			if err != nil {
				return err
			}
			req.SetBasicAuth(t.accountSID, t.authToken)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := t.http.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode >= 300 {
				return &apiError{status: resp.StatusCode, body: string(b)}
			}
			return nil
		})
	})
}
