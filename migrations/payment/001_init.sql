CREATE TABLE IF NOT EXISTS payments (
    id                 UUID PRIMARY KEY,
    tracking_number    TEXT NOT NULL,
    customer_id        TEXT NOT NULL,
    amount_minor       BIGINT NOT NULL CHECK (amount_minor > 0),
    currency           CHAR(3) NOT NULL,
    status             TEXT NOT NULL,
    provider_intent_id TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_tracking ON payments (tracking_number);

-- One row per processed Stripe event: makes webhook handling idempotent.
CREATE TABLE IF NOT EXISTS processed_webhook_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
