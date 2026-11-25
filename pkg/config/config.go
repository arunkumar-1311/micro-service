// Package config holds tiny helpers for reading 12-factor style env config.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func String(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Int(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func Duration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func List(key string, def []string) []string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ",")
	}
	return def
}
