// Package sender abstracts the SMS provider so Twilio can be swapped or faked.
package sender

import (
	"context"
	"log/slog"
)

type SMSSender interface {
	Send(ctx context.Context, to, body string) error
}

// Log is a dev/no-credentials fallback: it only logs (phone masked).
type Log struct{ log *slog.Logger }

func NewLog(log *slog.Logger) *Log { return &Log{log: log} }

func (l *Log) Send(_ context.Context, to, body string) error {
	l.log.Info("sms (log sender, not delivered)", "to", Mask(to), "body", body)
	return nil
}

// Mask hides all but the last 4 digits so phone numbers don't leak into logs.
func Mask(phone string) string {
	if len(phone) <= 4 {
		return "****"
	}
	return "****" + phone[len(phone)-4:]
}
