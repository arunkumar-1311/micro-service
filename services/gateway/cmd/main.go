package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"logistics/pkg/config"
	"logistics/pkg/httpx"
	"logistics/pkg/logger"
	"logistics/services/gateway/internal/proxy"
)

// Public API (all under /api/v1) -> internal service:
//
//	POST   /shipments                  shipment-service
//	GET    /shipments/:tracking        shipment-service
//	GET    /shipments/:tracking/events shipment-service
//	PATCH  /shipments/:tracking/status shipment-service
//	POST   /payments/intents           payment-service
//	GET    /payments/:id               payment-service
//	POST   /webhooks/stripe            payment-service (public, signature verified downstream)
func main() {
	log := logger.New("gateway")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shipment, err := proxy.New("shipment-service", config.String("SHIPMENT_URL", "http://localhost:8081"), log)
	must(err)
	payment, err := proxy.New("payment-service", config.String("PAYMENT_URL", "http://localhost:8082"), log)
	must(err)

	// /api/v1/x -> /v1/x
	strip := func(p string) string { return strings.Replace(p, "/api", "", 1) }

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), httpx.RequestID(), httpx.AccessLog(log))
	r.GET("/healthz", httpx.Health)

	api := r.Group("/api/v1")
	// TODO: auth (JWT) + rate limiting middleware on this group.
	api.Any("/shipments", shipment.Handler(strip))
	api.Any("/shipments/*rest", shipment.Handler(strip))
	api.Any("/payments", payment.Handler(strip))
	api.Any("/payments/*rest", payment.Handler(strip))
	// Webhooks must stay outside auth: Stripe authenticates via signature.
	api.POST("/webhooks/stripe", payment.Handler(strip))

	srv := &http.Server{Addr: ":" + config.String("PORT", "8080"), Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	log.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
