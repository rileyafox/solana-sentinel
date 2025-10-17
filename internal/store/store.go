package store

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"  
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

// New creates a pgx pool from DSN with sane defaults.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	// Pool sizing (override with PG_MAX_CONNS if needed)
	cfg.MaxConns = 10
	if v := os.Getenv("PG_MAX_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConns = int32(n)
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { if s != nil && s.pool != nil { s.pool.Close() } }

// Health pings the DB.
func (s *Store) Health(ctx context.Context) error {
	var one int
	return s.pool.QueryRow(ctx, "SELECT 1").Scan(&one)
}

// EnsureSchema bootstraps the minimal table + indexes used
func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS tx_events (
  signature  TEXT PRIMARY KEY,
  slot       BIGINT NOT NULL,
  err        JSONB NULL,
  logs       TEXT  NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS tx_events_slot_idx ON tx_events(slot DESC);
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname = 'public' AND indexname = 'tx_events_logs_gin'
  ) THEN
    EXECUTE 'CREATE INDEX tx_events_logs_gin ON tx_events USING gin (to_tsvector(''simple'', logs));';
  END IF;
END $$;`)
	return err
}

func (s *Store) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.pool.Query(ctx, sql, args...)
}


// UpsertTxEvent writes/updates a single event by signature.
func (s *Store) UpsertTxEvent(ctx context.Context, signature string, slot int64, errJSON string, logs string) error {
	const q = `
INSERT INTO tx_events (signature, slot, err, logs)
VALUES ($1, $2, $3::jsonb, $4)
ON CONFLICT (signature) DO UPDATE
  SET slot = EXCLUDED.slot,
      err  = EXCLUDED.err,
      logs = EXCLUDED.logs;
`
	_, err := s.pool.Exec(ctx, q, signature, slot, errJSON, logs)
	return err
}

// ListLatestEvents is handy for your REST read path (kept here for reuse).
type EventAPIRow struct {
	Signature string
	Slot      int64
	ErrText   *string
	Logs      string
	CreatedAt time.Time
}

func (s *Store) ListLatestEvents(ctx context.Context, limit int, programContains string, sinceSlot, untilSlot int64) ([]EventAPIRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	where := `1=1`
	args := make([]any, 0, 4)

	if programContains != "" {
		where += ` AND logs ILIKE '%'||$` + itoa(len(args)+1) + `||'%'`
		args = append(args, programContains)
	}
	if sinceSlot > 0 {
		where += ` AND slot >= $` + itoa(len(args)+1)
		args = append(args, sinceSlot)
	}
	if untilSlot > 0 {
		where += ` AND slot <= $` + itoa(len(args)+1)
		args = append(args, untilSlot)
	}
	args = append(args, limit)

	q := `
SELECT signature,
       slot,
       CASE WHEN err::text = 'null' THEN NULL ELSE err::text END AS err_text,
       logs,
       created_at
FROM tx_events
WHERE ` + where + `
ORDER BY slot DESC
LIMIT $` + itoa(len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EventAPIRow, 0, limit)
	for rows.Next() {
		var e EventAPIRow
		if err := rows.Scan(&e.Signature, &e.Slot, &e.ErrText, &e.Logs, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

/* ---------- Compatibility layer for existing code ---------- */

// TxRow and EventRow are kept so internal/backfill/parse can compile unchanged.
type TxRow struct {
	Signature string
	Slot      int64
	BlockTime *time.Time
	Fee       int64
	ErrJSON   []byte // JSONB
	RawJSON   []byte // JSONB (we'll stash this into tx_events.logs for now)
}

type EventRow struct {
	Kind       string
	Signature  string
	Slot       int64
	Account    *string
	Program    *string
	Amount     *string // numeric as string
	Mint       *string
	RawJSON    []byte
	OccurredAt time.Time
}

// InsertTransaction maps to tx_events; we keep err/raw for visibility.
func (s *Store) InsertTransaction(ctx context.Context, tx TxRow) error {
	errJSON := "null"
	if tx.ErrJSON != nil && len(tx.ErrJSON) > 0 {
		errJSON = string(tx.ErrJSON)
	}
	logs := ""
	if tx.RawJSON != nil && len(tx.RawJSON) > 0 {
		logs = string(tx.RawJSON)
	}
	return s.UpsertTxEvent(ctx, tx.Signature, tx.Slot, errJSON, logs)
}

// ReplaceEventsForSignature is a no-op placeholder until a structured events table exists.
func (s *Store) ReplaceEventsForSignature(ctx context.Context, _ string, _ []EventRow) error {
	// Intentionally no-op for now; add real table + UPSERTs when you introduce structured events.
	return nil
}

// Minimal struct + method to satisfy callers that list recent txs.
type ListTxsRow struct {
	Signature string
	Slot      int64
	Fee       int64
}

func (s *Store) ListRecentTxs(ctx context.Context, limit int32) ([]ListTxsRow, error) {
	if limit <= 0 {
		limit = 10
	}
	q := `
SELECT signature, slot
FROM tx_events
ORDER BY slot DESC
LIMIT $1;
`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListTxsRow
	for rows.Next() {
		var r ListTxsRow
		if err := rows.Scan(&r.Signature, &r.Slot); err != nil {
			return nil, err
		}
		// tx_events doesn't track fees; set 0 as a placeholder.
		r.Fee = 0
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

/* ---------- Helpers ---------- */

var ErrNotFound = errors.New("not found")

func itoa(i int) string { return strconv.Itoa(i) }
