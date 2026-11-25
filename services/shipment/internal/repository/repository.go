package repository

import (
	"context"

	"logistics/services/shipment/internal/domain"
)

// ShipmentRepository abstracts persistence so the service layer can be
// tested with a fake and the store swapped without touching business logic.
type ShipmentRepository interface {
	Create(ctx context.Context, s *domain.Shipment) error
	GetByTracking(ctx context.Context, tracking string) (*domain.Shipment, error)
	// UpdateStatus atomically moves the shipment to next and appends a
	// tracking event. It returns the updated shipment.
	UpdateStatus(ctx context.Context, tracking string, next domain.Status, location, note string) (*domain.Shipment, error)
	ListEvents(ctx context.Context, tracking string) ([]domain.TrackingEvent, error)
}
