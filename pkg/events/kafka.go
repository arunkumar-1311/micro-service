package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"

	"logistics/pkg/resilience"
)

type KafkaPublisher struct {
	w  *kafka.Writer
	cb *resilience.CircuitBreaker
}

func NewKafkaPublisher(brokers []string) *KafkaPublisher {
	return &KafkaPublisher{
		w: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Balancer:               &kafka.Hash{},
			RequiredAcks:           kafka.RequireAll,
			WriteTimeout:           5 * time.Second,
			AllowAutoTopicCreation: true,
		},
		cb: resilience.NewCircuitBreaker(5, 15*time.Second),
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic string, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	msg := kafka.Message{Topic: topic, Key: []byte(e.Key), Value: body}
	return p.cb.Execute(func() error {
		return resilience.Retry(ctx, resilience.DefaultRetry, func(ctx context.Context) error {
			return p.w.WriteMessages(ctx, msg)
		})
	})
}

func (p *KafkaPublisher) Close() error { return p.w.Close() }

type KafkaConsumer struct {
	r   *kafka.Reader
	log *slog.Logger
}

func NewKafkaConsumer(brokers []string, topic, group string, log *slog.Logger) *KafkaConsumer {
	return &KafkaConsumer{
		r:   kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic, GroupID: group, MinBytes: 1, MaxBytes: 10e6}),
		log: log,
	}
}

// Run commits an offset only after the handler finishes (at-least-once), so
// handlers must be idempotent (dedupe on Event.ID).
func (c *KafkaConsumer) Run(ctx context.Context, h Handler) error {
	for {
		m, err := c.r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var e Event
		if err := json.Unmarshal(m.Value, &e); err != nil {
			c.log.Error("dropping malformed event", "err", err, "offset", m.Offset) // TODO: DLQ
			_ = c.r.CommitMessages(ctx, m)
			continue
		}
		err = resilience.Retry(ctx, resilience.DefaultRetry, func(ctx context.Context) error { return h(ctx, e) })
		if err != nil {
			c.log.Error("handler failed after retries", "event", e.ID, "type", e.Type, "err", err) // TODO: DLQ
		}
		if err := c.r.CommitMessages(ctx, m); err != nil {
			return err
		}
	}
}

func (c *KafkaConsumer) Close() error { return c.r.Close() }
