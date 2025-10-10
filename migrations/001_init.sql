
-- Base schema (initial)
CREATE TABLE IF NOT EXISTS transactions (
  signature TEXT PRIMARY KEY,
  slot BIGINT NOT NULL,
  block_time TIMESTAMPTZ NULL,
  err JSONB NULL,
  fee BIGINT NOT NULL,
  raw JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  id BIGSERIAL PRIMARY KEY,
  kind TEXT NOT NULL,
  signature TEXT NOT NULL REFERENCES transactions(signature),
  slot BIGINT NOT NULL,
  account TEXT NULL,
  program TEXT NULL,
  amount NUMERIC NULL,
  mint TEXT NULL,
  raw JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_events_kind_time ON events(kind, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_account ON events(account);
CREATE INDEX IF NOT EXISTS idx_events_program ON events(program);

CREATE TABLE IF NOT EXISTS accounts (
  pubkey TEXT PRIMARY KEY,
  owner  TEXT NOT NULL,
  slot   BIGINT NOT NULL,
  lamports BIGINT NOT NULL,
  executable BOOLEAN NOT NULL,
  rent_epoch BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tx_slot ON transactions(slot DESC);
