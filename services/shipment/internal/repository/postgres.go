package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"logistics/services/shipment/internal/domain"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const shipmentCols = `id, tracking_number, customer_id, recipient_name, recipient_phone,
	origin_line1, origin_city, origin_state, origin_postal_code, origin_country,
	dest_line1, dest_city, dest_state, dest_postal_code, dest_country,
	weight_kg, status, created_at, updated_at`

func scan(row pgx.Row) (*domain.Shipment, error) {
	var s domain.Shipment
	err := row.Scan(&s.ID, &s.TrackingNumber, &s.CustomerID, &s.RecipientName, &s.RecipientPhone,
		&s.Origin.Line1, &s.Origin.City, &s.Origin.State, &s.Origin.PostalCode, &s.Origin.Country,
		&s.Destination.Line1, &s.Destination.City, &s.Destination.State, &s.Destination.PostalCode, &s.Destination.Country,
		&s.WeightKg, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &s, err
}

func (p *Postgres) Create(ctx context.Context, s *domain.Shipment) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `INSERT INTO shipments
		(tracking_number, customer_id, recipient_name, recipient_phone,
		 origin_line1, origin_city, origin_state, origin_postal_code, origin_country,
		 dest_line1, dest_city, dest_state, dest_postal_code, dest_country, weight_kg, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id, created_at, updated_at`,
		s.TrackingNumber, s.CustomerID, s.RecipientName, s.RecipientPhone,
		s.Origin.Line1, s.Origin.City, s.Origin.State, s.Origin.PostalCode, s.Origin.Country,
		s.Destination.Line1, s.Destination.City, s.Destination.State, s.Destination.PostalCode, s.Destination.Country,
		s.WeightKg, s.Status).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tracking_events (shipment_id, status, location, note)
		VALUES ($1,$2,$3,'shipment created')`, s.ID, s.Status, s.Origin.City); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) GetByTracking(ctx context.Context, tracking string) (*domain.Shipment, error) {
	return scan(p.pool.QueryRow(ctx, `SELECT `+shipmentCols+` FROM shipments WHERE tracking_number=$1`, tracking))
}

func (p *Postgres) UpdateStatus(ctx context.Context, tracking string, next domain.Status, location, note string) (*domain.Shipment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Row lock prevents two concurrent updates racing past the transition check.
	s, err := scan(tx.QueryRow(ctx, `SELECT `+shipmentCols+` FROM shipments WHERE tracking_number=$1 FOR UPDATE`, tracking))
	if err != nil {
		return nil, err
	}
	if !s.Status.CanTransitionTo(next) {
		return nil, domain.ErrInvalidTransition
	}
	if err = tx.QueryRow(ctx, `UPDATE shipments SET status=$1, updated_at=now() WHERE id=$2 RETURNING updated_at`,
		next, s.ID).Scan(&s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Status = next
	if _, err = tx.Exec(ctx, `INSERT INTO tracking_events (shipment_id, status, location, note) VALUES ($1,$2,$3,$4)`,
		s.ID, next, location, note); err != nil {
		return nil, err
	}
	return s, tx.Commit(ctx)
}

func (p *Postgres) ListEvents(ctx context.Context, tracking string) ([]domain.TrackingEvent, error) {
	rows, err := p.pool.Query(ctx, `SELECT e.id, e.shipment_id, e.status, e.location, e.note, e.occurred_at
		FROM tracking_events e JOIN shipments s ON s.id = e.shipment_id
		WHERE s.tracking_number=$1 ORDER BY e.occurred_at, e.id`, tracking)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TrackingEvent
	for rows.Next() {
		var e domain.TrackingEvent
		if err := rows.Scan(&e.ID, &e.ShipmentID, &e.Status, &e.Location, &e.Note, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
