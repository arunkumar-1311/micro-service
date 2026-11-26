package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"logistics/services/payment/internal/domain"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const cols = `id, tracking_number, customer_id, amount_minor, currency, status, provider_intent_id, created_at, updated_at`

func scan(row pgx.Row) (*domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(&p.ID, &p.TrackingNumber, &p.CustomerID, &p.AmountMinor, &p.Currency, &p.Status,
		&p.ProviderIntent, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &p, err
}

func (r *Postgres) Create(ctx context.Context, p *domain.Payment) error {
	return r.pool.QueryRow(ctx, `INSERT INTO payments (id, tracking_number, customer_id, amount_minor, currency, status)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING created_at, updated_at`,
		p.ID, p.TrackingNumber, p.CustomerID, p.AmountMinor, p.Currency, p.Status).Scan(&p.CreatedAt, &p.UpdatedAt)
}

func (r *Postgres) Get(ctx context.Context, id string) (*domain.Payment, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+cols+` FROM payments WHERE id=$1`, id))
}

func (r *Postgres) SetIntent(ctx context.Context, id, intentID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE payments SET provider_intent_id=$2, updated_at=now() WHERE id=$1`, id, intentID)
	return err
}

func (r *Postgres) MarkFailed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE payments SET status='FAILED', updated_at=now() WHERE id=$1 AND status='PENDING'`, id)
	return err
}

func (r *Postgres) ApplyWebhook(ctx context.Context, eventID, paymentID string, to domain.Status) (*WebhookResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `INSERT INTO processed_webhook_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return nil, err
	}
	p, err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM payments WHERE id=$1 FOR UPDATE`, paymentID))
	if err != nil {
		return nil, err // unknown payment: rolls back, event NOT recorded
	}
	if tag.RowsAffected() == 0 {
		return &WebhookResult{Payment: p, Duplicate: true}, nil
	}
	res := &WebhookResult{Payment: p}
	if p.Status.CanMoveTo(to) {
		if err = tx.QueryRow(ctx, `UPDATE payments SET status=$2, updated_at=now() WHERE id=$1 RETURNING updated_at`,
			paymentID, to).Scan(&p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Status, res.Changed = to, true
	}
	return res, tx.Commit(ctx)
}
