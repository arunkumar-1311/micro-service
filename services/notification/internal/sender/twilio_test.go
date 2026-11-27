package sender

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestTwilioRetriesTransientOnly(t *testing.T) {
	var calls atomic.Int32
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		if user != "AC1" || r.FormValue("To") != "+15551234567" {
			t.Errorf("bad request: user=%q to=%q", user, r.FormValue("To"))
		}
		if calls.Add(1) <= 2 {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	tw := NewTwilio("AC1", "tok", "+15550000000")
	tw.baseURL = srv.URL
	if err := tw.Send(context.Background(), "+15551234567", "hi"); err != nil || calls.Load() != 3 {
		t.Fatalf("5xx should be retried: err=%v calls=%d", err, calls.Load())
	}

	calls.Store(0)
	status = http.StatusBadRequest // invalid number: retrying is pointless
	if err := tw.Send(context.Background(), "+15551234567", "hi"); err == nil || calls.Load() != 1 {
		t.Fatalf("4xx must not be retried: err=%v calls=%d", err, calls.Load())
	}
}
