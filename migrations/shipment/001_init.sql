CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS shipments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracking_number    TEXT NOT NULL UNIQUE,
    customer_id        TEXT NOT NULL,
    recipient_name     TEXT NOT NULL,
    recipient_phone    TEXT NOT NULL,
    origin_line1       TEXT NOT NULL,
    origin_city        TEXT NOT NULL,
    origin_state       TEXT NOT NULL DEFAULT '',
    origin_postal_code TEXT NOT NULL,
    origin_country     CHAR(2) NOT NULL,
    dest_line1         TEXT NOT NULL,
    dest_city          TEXT NOT NULL,
    dest_state         TEXT NOT NULL DEFAULT '',
    dest_postal_code   TEXT NOT NULL,
    dest_country       CHAR(2) NOT NULL,
    weight_kg          NUMERIC(10,3) NOT NULL CHECK (weight_kg > 0),
    status             TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_shipments_customer ON shipments (customer_id);

CREATE TABLE IF NOT EXISTS tracking_events (
    id          BIGSERIAL PRIMARY KEY,
    shipment_id UUID NOT NULL REFERENCES shipments(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    location    TEXT NOT NULL DEFAULT '',
    note        TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tracking_events_shipment ON tracking_events (shipment_id, occurred_at);
