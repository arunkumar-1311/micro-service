# Logistics & Shipment Tracking — Microservices

Go · Gin · PostgreSQL · Redis · Kafka · gRPC · Stripe · SMS (Twilio)

## Layout

```
api/proto/            gRPC contracts (internal sync calls)
pkg/                  shared libs: config, logger, resilience (retry/breaker), events (Kafka), cache (Redis), httpx
services/
  gateway/            public edge: routing, request-id, per-upstream circuit breaker
  shipment/           domain → repository(interface) → service(interface) → handler
  payment/            Stripe intents + signed, idempotent webhook
  notification/       Kafka consumer → Twilio SMS
migrations/<svc>/     SQL per service (each service owns its DB)
deploy/               docker-compose for local infra
```

Each service: `cmd/` (wiring only) and `internal/{domain,repository,service,handler}`. Dependencies point inward through interfaces.

## Public API (via gateway, `:8080`)

| Method | Path | Service |
|---|---|---|
| POST | `/api/v1/shipments` | shipment |
| GET | `/api/v1/shipments/{tracking}` | shipment (Redis cache-aside) |
| GET | `/api/v1/shipments/{tracking}/events` | shipment |
| PATCH | `/api/v1/shipments/{tracking}/status` | shipment (state machine, emits Kafka event) |
| POST | `/api/v1/payments/intents` | payment |
| GET | `/api/v1/payments/{id}` | payment |
| POST | `/api/v1/webhooks/stripe` | payment |

## Run locally

```
docker compose -f deploy/docker-compose.yml up -d
go mod tidy
go run ./services/shipment/cmd     # :8081
go run ./services/payment/cmd      # :8082 (needs STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET)
go run ./services/notification/cmd # :8083 (TWILIO_ACCOUNT_SID/AUTH_TOKEN/FROM_NUMBER; logs SMS if unset)
go run ./services/gateway/cmd      # :8080
```
