
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{
	pool *pgxpool.Pool
}

func New(dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { if s.pool != nil { s.pool.Close() } }

func (s *Store) Health(ctx context.Context) error {
	var one int
	return s.pool.QueryRow(ctx, "SELECT 1").Scan(&one)
}

// ---- Rows used by parser/backfill ----
type TxRow struct {
	Signature   string
	Slot        int64
	BlockTime   *time.Time
	Fee         int64
	ErrJSON     []byte // JSONB
	RawJSON     []byte // JSONB
}

type EventRow struct {
	Kind      string
	Signature string
	Slot      int64
	Account   *string
	Program   *string
	Amount    *string // numeric as string to avoid precision loss; NULL if not applicable
	Mint      *string
	RawJSON   []byte
	OccurredAt time.Time
}

// InsertTransaction upserts a transaction keyed by signature.
func (s *Store) InsertTransaction(ctx context.Context, tx TxRow) error {
	const q = `
	INSERT INTO transactions (signature, slot, block_time, err, fee, raw)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (signature) DO UPDATE
	SET slot = EXCLUDED.slot,
	    block_time = EXCLUDED.block_time,
	    err = EXCLUDED.err,
	    fee = EXCLUDED.fee,
	    raw = EXCLUDED.raw;
	`
	_, err := s.pool.Exec(ctx, q, tx.Signature, tx.Slot, tx.BlockTime, tx.ErrJSON, tx.Fee, tx.RawJSON)
	return err
}

// ReplaceEventsForSignature deletes existing events for a tx and inserts new ones (idempotent per signature).
func (s *Store) ReplaceEventsForSignature(ctx context.Context, sig string, evs []EventRow) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return err }
	defer func(){
		_ = tx.Rollback(ctx)
	}()
	_, err = tx.Exec(ctx, "DELETE FROM events WHERE signature=$1", sig)
	if err != nil { return err }
	const ins = `
	INSERT INTO events (kind, signature, slot, account, program, amount, mint, raw, occurred_at)
	VALUES ($1,$2,$3,$4,$5,CAST(NULLIF($6,'') AS NUMERIC),$7,$8,$9);
	`
	for _, e := range evs {
		amountStr := ""
		if e.Amount != nil { amountStr = *e.Amount }
		_, err = tx.Exec(ctx, ins, e.Kind, e.Signature, e.Slot, e.Account, e.Program, amountStr, e.Mint, e.RawJSON, e.OccurredAt)
		if err != nil { return err }
	}
	return tx.Commit(ctx)
}

// UpsertAccount snapshot.
func (s *Store) UpsertAccount(ctx context.Context, pubkey, owner string, slot int64, lamports int64, executable bool, rentEpoch int64) error {
	const q = `
	INSERT INTO accounts (pubkey, owner, slot, lamports, executable, rent_epoch, updated_at)
	VALUES ($1,$2,$3,$4,$5,$6, NOW())
	ON CONFLICT (pubkey) DO UPDATE
	SET owner=EXCLUDED.owner, slot=EXCLUDED.slot, lamports=EXCLUDED.lamports,
	    executable=EXCLUDED.executable, rent_epoch=EXCLUDED.rent_epoch, updated_at=NOW();
	`
	_, err := s.pool.Exec(ctx, q, pubkey, owner, slot, lamports, executable, rentEpoch)
	return err
}

type ListTxsRow struct {
	Signature string
	Slot      int64
	Fee       int64
}

func (s *Store) ListRecentTxs(ctx context.Context, limit int32) ([]ListTxsRow, error) {
	if limit <= 0 { limit = 10 }
	rows, err := s.pool.Query(ctx, "SELECT signature, slot, fee FROM transactions ORDER BY slot DESC LIMIT $1", limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []ListTxsRow
	for rows.Next() {
		var r ListTxsRow
		if err := rows.Scan(&r.Signature, &r.Slot, &r.Fee); err != nil { return nil, err }
		out = append(out, r)
	}
	if rows.Err() != nil { return nil, rows.Err() }
	return out, nil
}

var ErrNotFound = errors.New("not found")
