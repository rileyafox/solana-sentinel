# Solana Sentinel — Real‑Time Solana Indexer (Go + gRPC + Postgres)

**Solana Sentinel** is a production‑style backend that ingests **live Solana data** via WebSocket & JSON‑RPC, normalizes it into **Postgres**, and exposes **gRPC** (with REST via grpc‑gateway) for querying and streaming events in real time. It’s built to showcase backend engineering skills for roles like **Coinbase**.

> This repository is delivered as a scaffold you can incrementally extend. The roadmap is split into phases, with clear demo steps and “definition of done.”

---

## Highlights
- **Go 1.22+** service with clean, modular layout (`cmd/`, `internal/`).
- **Protobuf-first contracts** (`buf`) → gRPC stubs + REST gateway.
- **Solana clients** (HTTP JSON‑RPC + WebSocket) with retry/backoff stubs.
- **Postgres schema** (migrations) for transactions, events, accounts.
- **Docker Compose** for local dev (API, worker, Postgres, Redis, Prometheus).
- **Observability** stubs (Prom metrics, OpenTelemetry hooks).
- **Security & reliability** scaffolding (validation, rate‑limit entrypoints).

---

---

## Tech Stack
- **Language:** Go 1.22+
- **API:** gRPC + grpc‑gateway (REST) + OpenAPI
- **Schema/Migrations:** Postgres (SQL), `migrations/`
- **Build/Codegen:** `buf` for protobuf
- **Runtime:** Docker Compose for local dev
- **Cache/Dedupe (future):** Redis
- **Obs (future):** Prometheus, OpenTelemetry

---

## Repository Layout
```
solana-sentinel/
  api/
    v1/
      sentinel.proto          # Contracts (gRPC + REST via gateway)
  buf.gen.yaml
  buf.yaml
  cmd/
    sentinel-api/
      main.go                 # gRPC/REST server entrypoint
    sentinel-worker/
      main.go                 # Backfill / jobs entrypoint
  internal/
    backfill/                 # Historical ingestion (stubs)
      backfill.go
    config/                   # Configuration loader (stubs)
      config.go
    gateway/                  # grpc-gateway setup (stubs)
      gateway.go
    ingest/                   # Live WS ingestion (stubs)
      ingest.go
    observability/            # Prom/OTel hooks (stubs)
      observability.go
    parse/                    # Normalizers/parsers (stubs)
      parse.go
    rpc/                      # Solana RPC clients (stubs)
      http.go
      ws.go
    service/                  # Implements the gRPC service
      sentinel.go
    store/                    # DB access (stubs)
      store.go
  migrations/
    001_init.sql              # Base schema
  docker/
    docker-compose.yaml
  .github/
    workflows/
      ci.yml                  # Lint/build/codegen CI
  .gitignore
  LICENSE
  Makefile
  README.md
  go.mod
```

---

## Quickstart

### 1) Prerequisites
- Go 1.22+
- Docker + Docker Compose
- `buf` (protobuf tool): <https://docs.buf.build/installation>

### 2) Clone & Generate
```bash
buf mod update
buf generate
```

### 3) Start the stack
```bash
docker compose -f docker/docker-compose.yaml up -d --build
make migrate
```

### 4) Run services
```bash
# In one shell
go run ./cmd/sentinel-api

# In another shell
go run ./cmd/sentinel-worker --help
```

> The API server exposes gRPC on `:8080` and REST on `:8081` (grpc‑gateway). Health endpoint: `http://localhost:8081/health` (stub).

---

##  Configuration
Environment variables (with sensible defaults) are loaded by `internal/config`:

| Variable | Default | Description |
|---|---|---|
| `SOLANA_HTTP_URL` | `https://api.devnet.solana.com` | Solana JSON‑RPC HTTP endpoint |
| `SOLANA_WS_URL`   | `wss://api.devnet.solana.com`    | Solana WebSocket endpoint |
| `DATABASE_URL`    | `postgres://postgres:postgres@localhost:5433/sentinel?sslmode=disable` | Postgres DSN |
| `REDIS_ADDR`      | `localhost:6379`                 | Redis (future dedupe/cache) |
| `LOG_LEVEL`       | `info`                           | `debug`, `info`, `warn`, `error` |

Set via `.env` or your shell. Docker Compose supplies local values.

---

## 🗃 Database Schema (initial)
See `migrations/001_init.sql` for details. Core tables:

- **transactions**: signature (PK), slot, block_time, err (JSONB), fee, raw (JSONB)
- **events**: id (PK), kind, signature (FK), slot, account, program, amount, mint, raw (JSONB), occurred_at
- **accounts**: pubkey (PK), owner, slot, lamports, executable, rent_epoch, updated_at

> Keep `raw` JSON to enable re-parsing without re-fetching RPC. Add indexes as usage patterns emerge.

### Apply migrations
```bash
make migrate
```

---

## 🔌 Protobuf & gRPC
The initial contract exposes two methods (scaffold only):
- `GetTransactions` — list recent transactions for an account/program.
- `StreamEvents` — server streaming of normalized events.

Generate stubs:
```bash
buf generate
```

OpenAPI is configured in `buf.gen.yaml` (generated under `api/v1/`).

---

## Testing (starter)
- Unit tests should live under each package with `_test.go` suffix.
- Recommended tools: `go test ./...`, `golangci-lint` (CI already configured).

CI runs:
- `buf lint`
- `go vet`
- `go build` for both commands
- (Optionally) `golangci-lint run`

---

## Observability
- `/metrics` (Prometheus) — stubbed for now.
- OpenTelemetry hooks (tracing/logs) wired in `internal/observability` as no‑ops; upgrade in Phase 8.

---

## Security Notes
- Validate user input (pubkeys, program IDs).
- Add rate limiting to REST endpoints before exposing publicly.
- Consider API keys for REST (simple HMAC header) in Phase 8.

---

## Roadmap (Phases)
1. **Repo & Contracts** — scaffold (this commit).
2. **Solana RPC Client** — HTTP+WS with retry/backoff and context cancellation.
3. **Storage** — migrations + store layer with idempotent upserts.
4. **Historical Backfill** — `getSignaturesForAddress` → `getTransaction` normalization.
5. **gRPC/REST API** — query + server streaming.
6. **Live Ingestor** — WS logs/accounts + dedupe + fanout.
7. **Program Decoders** — system transfers, SPL tokens, NFTs, Jupiter swaps.
8. **Analytics & Views** — materialized views + endpoints + dashboards.
9. **Reliability & Security** — OTel, rate limiting, API keys, load tests.
10. **Scale** — Kafka/NATS, partitioned tables, K8s/Helm.

Each phase should add: code, tests, migrations, **README section** with demo commands and screenshots/GIFs.

---

> Designed a production‑style Go backend that ingests live Solana data via WebSocket & JSON‑RPC, normalizes transactions/events into Postgres, and exposes gRPC/REST for real‑time queries and streaming. Built protobuf‑first contracts, containerized dev stack, and observability hooks. Roadmapped program‑aware decoders and analytics.

---

