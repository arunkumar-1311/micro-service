package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"logistics/pkg/cache"
	"logistics/pkg/events"
	"logistics/services/shipment/internal/domain"
	"logistics/services/shipment/internal/repository"
)

type CreateInput struct {
	CustomerID     string
	RecipientName  string
	RecipientPhone string
	Origin         domain.Address
	Destination    domain.Address
	WeightKg       float64
}

type ShipmentService interface {
	Create(ctx context.Context, in CreateInput) (*domain.Shipment, error)
	Get(ctx context.Context, tracking string) (*domain.Shipment, error)
	UpdateStatus(ctx context.Context, tracking string, next domain.Status, location, note string) (*domain.Shipment, error)
	Events(ctx context.Context, tracking string) ([]domain.TrackingEvent, error)
}

type shipmentService struct {
	repo  repository.ShipmentRepository
	cache cache.Cache
	pub   events.Publisher
	log   *slog.Logger
}

func New(repo repository.ShipmentRepository, c cache.Cache, pub events.Publisher, log *slog.Logger) ShipmentService {
	return &shipmentService{repo: repo, cache: c, pub: pub, log: log}
}

const cacheTTL = 5 * time.Minute

func cacheKey(tracking string) string { return "shipment:" + tracking }

func (s *shipmentService) Create(ctx context.Context, in CreateInput) (*domain.Shipment, error) {
	if in.WeightKg <= 0 {
		return nil, fmt.Errorf("%w: weight_kg must be > 0", domain.ErrValidation)
	}
	tn, err := newTrackingNumber()
	if err != nil {
		return nil, err
	}
	sh := &domain.Shipment{
		TrackingNumber: tn, CustomerID: in.CustomerID, RecipientName: in.RecipientName,
		RecipientPhone: in.RecipientPhone, Origin: in.Origin, Destination: in.Destination,
		WeightKg: in.WeightKg, Status: domain.StatusCreated,
	}
	if err := s.repo.Create(ctx, sh); err != nil {
		return nil, err
	}
	s.publish(ctx, events.ShipmentCreated, sh)
	return sh, nil
}

// Get is cache-aside. Redis failures degrade to a DB read rather than failing.
func (s *shipmentService) Get(ctx context.Context, tracking string) (*domain.Shipment, error) {
	if b, err := s.cache.Get(ctx, cacheKey(tracking)); err == nil {
		var sh domain.Shipment
		if json.Unmarshal(b, &sh) == nil {
			return &sh, nil
		}
	} else if !errors.Is(err, cache.ErrMiss) {
		s.log.Warn("cache read failed, falling back to db", "err", err)
	}
	sh, err := s.repo.GetByTracking(ctx, tracking)
	if err != nil {
		return nil, err
	}
	s.store(ctx, sh)
	return sh, nil
}

func (s *shipmentService) UpdateStatus(ctx context.Context, tracking string, next domain.Status, location, note string) (*domain.Shipment, error) {
	if !next.Valid() {
		return nil, fmt.Errorf("%w: unknown status %q", domain.ErrValidation, next)
	}
	sh, err := s.repo.UpdateStatus(ctx, tracking, next, location, note)
	if err != nil {
		return nil, err
	}
	if err := s.cache.Delete(ctx, cacheKey(tracking)); err != nil {
		s.log.Warn("cache invalidate failed", "err", err, "tracking", tracking)
	}
	s.publish(ctx, events.ShipmentStatusChanged, sh)
	return sh, nil
}

func (s *shipmentService) Events(ctx context.Context, tracking string) ([]domain.TrackingEvent, error) {
	if _, err := s.Get(ctx, tracking); err != nil {
		return nil, err
	}
	return s.repo.ListEvents(ctx, tracking)
}

func (s *shipmentService) store(ctx context.Context, sh *domain.Shipment) {
	if b, err := json.Marshal(sh); err == nil {
		if err := s.cache.Set(ctx, cacheKey(sh.TrackingNumber), b, cacheTTL); err != nil {
			s.log.Warn("cache write failed", "err", err)
		}
	}
}

// publish is best-effort here. TODO: replace with a transactional outbox so a
// Kafka outage can never lose an event after the DB commit succeeded.
func (s *shipmentService) publish(ctx context.Context, typ string, sh *domain.Shipment) {
	e, err := events.New(typ, sh.TrackingNumber, sh)
	if err == nil {
		err = s.pub.Publish(ctx, events.TopicShipment, e)
	}
	if err != nil {
		s.log.Error("publish failed", "type", typ, "tracking", sh.TrackingNumber, "err", err)
	}
}

func newTrackingNumber() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 10)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return "LG" + string(b), nil
}
