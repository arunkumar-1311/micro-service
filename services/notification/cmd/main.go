package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"logistics/pkg/cache"
	"logistics/pkg/config"
	"logistics/pkg/events"
	"logistics/pkg/httpx"
	"logistics/pkg/logger"
	"logistics/services/notification/internal/sender"
	"logistics/services/notification/internal/service"
)

func main() {
	log := logger.New("notification-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var sms sender.SMSSender
	sid, token, from := os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"), os.Getenv("TWILIO_FROM_NUMBER")
	if sid != "" && token != "" && from != "" {
		sms = sender.NewTwilio(sid, token, from)
	} else {
		log.Warn("Twilio credentials not set: SMS will only be logged")
		sms = sender.NewLog(log)
	}

	svc := service.New(sms, cache.NewRedis(config.String("REDIS_ADDR", "localhost:6379")), log)

	brokers := config.List("KAFKA_BROKERS", []string{"localhost:9092"})
	var wg sync.WaitGroup
	for _, topic := range []string{events.TopicShipment, events.TopicPayment} {
		c := events.NewKafkaConsumer(brokers, topic, "notification-service", log)
		wg.Add(1)
		go func(topic string) {
			defer wg.Done()
			defer c.Close()
			if err := c.Run(ctx, svc.Handle); err != nil {
				log.Error("consumer stopped", "topic", topic, "err", err)
				stop() // let the orchestrator restart us rather than run half-deaf
			}
		}(topic)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", httpx.Health)
	srv := &http.Server{Addr: ":" + config.String("PORT", "8083"), Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	log.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server", "err", err)
	}
	wg.Wait()
}
