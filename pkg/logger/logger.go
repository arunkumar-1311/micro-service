package logger

import (
	"log/slog"
	"os"
)

// New returns a JSON structured logger tagged with the service name.
func New(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
}
