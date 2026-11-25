package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"logistics/pkg/cache"
	"logistics/pkg/config"
	"logistics/pkg/events"
	"logistics/pkg/httpx"
	"logistics/pkg/logger"
	"logistics/services/shipment/internal/handler"
	"logistics/services/shipment/internal/repository"
	"logistics/services/shipment/internal/service"
)

func main() {
	log := logger.New("shipment-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, config.String("DATABASE_URL", "postgres://logistics:logistics@localhost:5432/shipment?sslmode=disable"))
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	pub := events.NewKafkaPublisher(config.List("KAFKA_BROKERS", []string{"localhost:9092"}))
	defer pub.Close()
	rc := cache.NewRedis(config.String("REDIS_ADDR", "localhost:6379"))

	svc := service.New(repository.NewPostgres(pool), rc, pub, log)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), httpx.RequestID(), httpx.AccessLog(log))
	handler.New(svc).Register(r)

	srv := &http.Server{Addr: ":" + config.String("PORT", "8081"), Handler: r, ReadHeaderTimeout: 5 * time.Second}
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
