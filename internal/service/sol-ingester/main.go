package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

type wsReq struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// Solana log notification (trimmed)
type logNoti struct {
	Params struct {
		Result struct {
			Context struct {
				Slot uint64 `json:"slot"`
			} `json:"context"`
			Value struct {
				Signature string   `json:"signature"`
				Err       any      `json:"err"`
				Logs      []string `json:"logs"`
			} `json:"value"`
		} `json:"result"`
		Subscription int `json:"subscription"`
	} `json:"params"`
}

var (
	ingested = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sentinel_ingested_events_total",
		Help: "Total Solana log events ingested (pre-dedupe).",
	})
	deduped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sentinel_deduped_events_total",
		Help: "Events dropped due to dedupe.",
	})
	published = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sentinel_published_events_total",
		Help: "Events published to Redis stream.",
	})
	reconnects = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sentinel_ws_reconnects_total",
		Help: "WebSocket reconnects.",
	})
)

func mustEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Accepts REDIS_URL like redis://host:6379[/db]
func mustRedisClient(raw string) *redis.Client {
	if raw == "" {
		raw = "redis://redis:6379/0"
	}
	opt, err := redis.ParseURL(raw)
	if err != nil {
		if !strings.HasPrefix(raw, "redis://") {
			if u, e := url.Parse("redis://" + raw); e == nil {
				opt, err = redis.ParseURL(u.String())
			}
		}
	}
	if err != nil {
		log.Fatalf("invalid REDIS_URL %q: %v", raw, err)
	}
	return redis.NewClient(opt)
}

func main() {
	prometheus.MustRegister(ingested, deduped, published, reconnects)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		addr := mustEnv("PROM_ADDR", ":9102")
		log.Printf("Prometheus at %s/metrics", addr)
		_ = http.ListenAndServe(addr, nil)
	}()

	ctx := context.Background()
	rdb := mustRedisClient(os.Getenv("REDIS_URL"))
	defer rdb.Close()

	wsURL := mustEnv("SOLANA_WS_URL", "wss://api.mainnet-beta.solana.com")
	commitment := mustEnv("SOLANA_COMMITMENT", "confirmed")
	programs := splitCSV(os.Getenv("SUBSCRIBE_PROGRAMS"))
	accounts := splitCSV(os.Getenv("SUBSCRIBE_ACCOUNTS"))
	dedupeTTL := time.Duration(envInt("REDIS_DEDUPE_TTL_SEC", 86400)) * time.Second

	for {
		if err := runOnce(ctx, rdb, wsURL, commitment, programs, accounts, dedupeTTL); err != nil {
			log.Printf("stream error: %v", err)
			reconnects.Inc()
			time.Sleep(backoff(err))
		}
	}
}

func runOnce(ctx context.Context, rdb *redis.Client, wsURL, commitment string, programs, accounts []string, dedupeTTL time.Duration) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		ReadBufferSize:   1 << 20,
		WriteBufferSize:  1 << 16,
	}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Build filter param: either "all" or {"mentions":[...]}
	var filter any = "all"
	if len(programs) > 0 {
		filter = map[string]any{"mentions": programs}
	} else if len(accounts) > 0 {
		filter = map[string]any{"mentions": accounts}
	}

	// Second param is opts object (commitment etc.)
	opts := map[string]any{"commitment": commitment}

	req := wsReq{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "logsSubscribe",
		Params:  []any{filter, opts},
	}
	if err := conn.WriteJSON(req); err != nil {
		return err
	}

	// Keepalive
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for range t.C {
			_ = conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
		}
	}()

	conn.SetReadLimit(10 << 20)
	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var noti logNoti
		if err := json.Unmarshal(msg, &noti); err != nil {
			// ignore non-notifications (e.g. subscribe ack)
			continue
		}
		ingested.Inc()

		sig := noti.Params.Result.Value.Signature
		slot := noti.Params.Result.Context.Slot
		if sig == "" {
			continue
		}

		// Dedupe on signature+slot
		key := "dedupe:" + sig + ":" + itoa(slot)
		ok, err := rdb.SetNX(ctx, key, "1", dedupeTTL).Result()
		if err != nil {
			log.Printf("redis dedupe err: %v", err)
			continue
		}
		if !ok {
			deduped.Inc()
			continue
		}

		// Publish to Redis Stream
		fields := map[string]any{
			"slot":      slot,
			"signature": sig,
			"err":       toJSON(noti.Params.Result.Value.Err),
			"logs":      strings.Join(noti.Params.Result.Value.Logs, "\n"),
			"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		}
		if _, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: "sol:logs",
			Values: fields,
			MaxLen: 100000, // rolling buffer
		}).Result(); err != nil {
			log.Printf("redis xadd err: %v", err)
			continue
		}
		published.Inc()
		log.Printf("published signature=%s slot=%d", sig, slot)
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func itoa(u uint64) string { return strconv.FormatUint(u, 10) }

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func backoff(_ error) time.Duration { return 3 * time.Second }
