Solana Sentinel is a production-grade backend that ingests live Solana mainnet data via WebSocket and JSON-RPC, buffers it in Redis Streams, persists normalized events to Postgres, and exposes both gRPC and REST APIs (via grpc-gateway) for real-time querying and monitoring.

Built to demonstrate backend, system design, and observability engineering — fully containerized with Docker Compose.

Highlights

Go 1.23+ modular architecture (cmd/, internal/)

Solana mainnet ingestion via WebSocket + retry/reconnect logic

Postgres storage with idempotent upserts (tx_events table)

Redis Streams for buffering and deduplication between services

gRPC + REST API powered by buf + grpc-gateway

Prometheus metrics for ingestion, storage, and API throughput

System design patterns for decoupling, resilience, and observability

Architecture Overview
             ┌──────────────────────────┐
             │   Solana Mainnet WS/API  │
             └───────────┬──────────────┘
                         │
                 (ingest via WebSocket)
                         ▼
               ┌──────────────────┐
               │ sol-ingester     │
               │ (cmd/sol-ingester) │
               └──────────┬────────┘
                         │
                  Publishes logs
                         ▼
               ┌──────────────────┐
               │ Redis Stream     │
               │ key: sol:logs    │
               └──────────┬────────┘
                         │
                Worker consumes + upserts
                         ▼
               ┌──────────────────┐
               │ sentinel-api     │
               │ (cmd/sentinel-api) │
               └──────────┬────────┘
                         │
                  Inserts into
                         ▼
               ┌──────────────────┐
               │ PostgreSQL       │
               │ table: tx_events │
               └──────────┬────────┘
                         │
                         ▼
             ┌────────────────────────────┐
             │ REST /v1/events/latest     │
             │ gRPC, Health, Metrics (/metrics) │
             └────────────────────────────┘

Tech Stack
Language	Go 1.23
API	gRPC + REST via grpc-gateway
Database	PostgreSQL 16
Cache / Buffer	Redis Streams
Metrics	Prometheus
Build / Codegen	buf for Protobufs
Runtime	Docker Compose


Repository Layout
solana-sentinel/
├── api/                  # gRPC + REST contracts
│   ├── proto/            # .proto definitions
│   └── gen/              # buf-generated stubs
│
├── cmd/                  # Service entrypoints
│   ├── sentinel-api/     # API + worker + metrics
│   │   └── main.go
│   └── sol-ingester/     # Solana WS log ingester
│       └── main.go
│
├── internal/             # Core logic
│   ├── api/              # REST/gRPC handlers (v1/events/latest)
│   ├── gateway/          # grpc-gateway setup
│   ├── observability/    # Prometheus + OTel stubs
│   ├── rpc/              # Solana JSON-RPC & WS clients
│   ├── stream/           # Redis stream helpers
│   ├── store/            # Postgres schema & queries
│   └── worker/           # Redis→Postgres consumer
│
├── docker/               # Deployment configs
│   ├── docker-compose.yaml
│   └── prometheus.yml
│
├── migrations/           # SQL schema
│   └── 001_init.sql
│
├── Makefile              # Local tasks (build, migrate)
└── README.md

Configuration

Environment variables (loaded from Docker Compose or .env):

Variable	Default	Description
SOLANA_WS_URL	wss://api.mainnet-beta.solana.com	WebSocket RPC endpoint
SOLANA_COMMITMENT	confirmed	Commitment level for stream data
SUBSCRIBE_PROGRAMS	TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA	Program IDs to monitor
REDIS_URL	redis://redis:6379/0	Redis connection
DATABASE_URL	postgres://postgres:postgres@db:5432/sentinel?sslmode=disable	Postgres DSN
GRPC_ADDR	:8081	gRPC bind address
REST_ADDR	:8080	REST gateway address
METRICS_ADDR	:9102	Prometheus metrics address

Quickstart (Mainnet)
1) Build & Run
docker compose -f docker/docker-compose.yaml up -d --build

2) Check services
docker compose ps

3️) Verify ingestion
# Redis stream
docker exec -it solana-sentinel-redis redis-cli XLEN sol:logs
# Postgres rows
docker exec -it solana-sentinel-db psql -U postgres -d sentinel \
  -c "SELECT count(*) FROM tx_events;"

4️) Call API
curl http://localhost:8080/v1/health
curl "http://localhost:8080/v1/events/latest?n=5"


You’ll see recent Solana transactions with decoded logs, slots, and timestamps.

Testing & Stress Scenarios
A) HTTP Load Testing (PowerShell)
1..500 | % { Invoke-WebRequest "http://localhost:8080/v1/events/latest?n=100" | Out-Null }

B) Dockerized Load Generator (Unix)
docker run --rm --network solana-sentinel_docker_default \
  rakyll/hey -z 60s -c 100 \
  http://solana-sentinel-api:8080/v1/events/latest?n=100


Expected:

p95 latency < 300 ms

No 5xx errors

count stable across runs

C) Monitor Ingestion Flow

Check Redis → Postgres pipeline health:

# Redis backlog
docker exec -it solana-sentinel-redis redis-cli XLEN sol:logs

# Recent Postgres inserts
docker exec -it solana-sentinel-db psql -U postgres -d sentinel \
  -c "SELECT date_trunc('second', created_at), count(*) FROM tx_events GROUP BY 1 ORDER BY 1 DESC LIMIT 10;"


Rows steadily increasing = worker is keeping up
Redis XLEN growing unbounded = worker lagging or DB bottleneck

D) Prometheus Metrics
Component	Endpoint	Key Metrics
API + Worker	http://localhost:9102/metrics	sentinel_events_emitted_total, sentinel_pg_errors_total, latency histograms
Ingester	http://localhost:9103/metrics	sentinel_ingested_events_total, sentinel_ws_reconnects_total, sentinel_redis_publish_total

Use the bundled Prometheus (http://localhost:9090) to visualize metrics and alert thresholds.

E) Scale & Fault Tolerance

Spin up multiple ingesters:

docker compose up -d --scale sol-ingester=3


Restart Redis or Postgres to confirm resilience:

docker restart solana-sentinel-redis
docker restart solana-sentinel-db


Next Steps (Future Work)

Add structured program decoders (e.g., SPL Token, Jupiter swaps)

Implement API filtering (program, slot range, amount)

Add Grafana dashboard and Prometheus alerts