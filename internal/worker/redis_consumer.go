package worker

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// RunRedisToPostgres consumes Redis stream events from "sol:logs" and
// writes them into Postgres table `tx_events` with an idempotent upsert.
func RunRedisToPostgres(ctx context.Context) error {
	// --- Redis client ---
	redisURL := getenv("REDIS_URL", "redis://redis:6379/0")
	ropt, err := redis.ParseURL(redisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(ropt)
	defer rdb.Close()

	// --- Postgres pool ---
	pgURL := getenv("DATABASE_URL", "postgres://postgres:postgres@db:5432/sentinel?sslmode=disable")
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Ensure table exists (safe if already created)
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS tx_events(
  signature  TEXT PRIMARY KEY,
  slot       BIGINT NOT NULL,
  err        JSONB NULL,
  logs       TEXT  NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}

	lastID := getenv("REDIS_FROM", "$") // "$" = only new items; "0-0" = backfill
	log.Printf("[worker] redis->pg starting at ID=%s", lastID)

	// Main read loop
	for ctx.Err() == nil {
		records, err := rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{"sol:logs", lastID},
			Count:   200,
			Block:   2 * time.Second, // wait for new items
		}).Result()

		if err == redis.Nil {
			continue // no new items within Block timeout
		}
		if err != nil {
			log.Printf("[worker] XREAD error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range records {
			for _, msg := range stream.Messages {
				sig := sval(msg.Values["signature"])
				if sig == "" {
					// Skip malformed entries
					lastID = msg.ID
					continue
				}

				// slot may be string or number depending on producer; normalize to int64
				var slotInt int64
				if s := sval(msg.Values["slot"]); s != "" {
					if n, err := strconv.ParseInt(s, 10, 64); err == nil {
						slotInt = n
					}
				}
				errJSON := sval(msg.Values["err"])  // "null" or JSON string
				logs := sval(msg.Values["logs"])    // joined lines

				// Idempotent upsert by signature
				_, uerr := pool.Exec(ctx, `
INSERT INTO tx_events (signature, slot, err, logs)
VALUES ($1, $2, $3::jsonb, $4)
ON CONFLICT (signature) DO UPDATE
  SET slot = EXCLUDED.slot,
      err  = EXCLUDED.err,
      logs = EXCLUDED.logs
`, sig, slotInt, errJSON, logs)
				if uerr != nil {
					log.Printf("[worker] upsert error (sig=%s): %v", sig, uerr)
					// do not advance lastID on failure; try again next read
					continue
				}

				lastID = msg.ID
			}
		}
	}

	return ctx.Err()
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func sval(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}
