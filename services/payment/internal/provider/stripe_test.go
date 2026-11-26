package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"logistics/services/payment/internal/domain"
)

func sign(secret, body string, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "." + body))
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

func TestParseWebhook(t *testing.T) {
	s := NewStripe("sk", "whsec_test")
	body := `{"id":"evt_1","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","metadata":{"payment_id":"p-1"}}}}`

	ev, err := s.ParseWebhook([]byte(body), sign("whsec_test", body, time.Now()))
	if err != nil || ev.ID != "evt_1" || ev.PaymentID != "p-1" || ev.IntentID != "pi_1" {
		t.Fatalf("ev=%+v err=%v", ev, err)
	}

	cases := map[string]string{
		"wrong secret": sign("other", body, time.Now()),
		"stale":        sign("whsec_test", body, time.Now().Add(-time.Hour)),
		"empty":        "",
		"tampered":     sign("whsec_test", body+" ", time.Now()),
	}
	for name, hdr := range cases {
		if _, err := s.ParseWebhook([]byte(body), hdr); !errors.Is(err, domain.ErrInvalidSignature) {
			t.Errorf("%s: want ErrInvalidSignature, got %v", name, err)
		}
	}
}
