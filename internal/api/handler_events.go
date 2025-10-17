package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/rileyafox/solana-sentinel/internal/store"
)

// Inject the Store from main at startup.
var dbStore *store.Store

// SetStore allows main() to wire the shared store instance.
func SetStore(s *store.Store) { dbStore = s }

type EventRow struct {
	Signature string  `json:"signature"`
	Slot      int64   `json:"slot"`
	Err       *string `json:"err"`        // raw JSON as string; null shows as null
	Logs      string  `json:"logs"`
	CreatedAt string  `json:"created_at"`
}

func LatestEventsHandler(w http.ResponseWriter, r *http.Request) {
	if dbStore == nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	// Query params
	n := clamp(intFrom(r, "n", 50), 1, 500)
	prog := strings.TrimSpace(r.URL.Query().Get("program_contains"))
	since := int64From(r, "since_slot", 0)
	until := int64From(r, "until_slot", 0)

	// Build WHERE
	where := "1=1"
	args := []any{}
	if prog != "" {
		where += " AND logs ILIKE '%'||$" + itoa(len(args)+1) + "||'%'"
		args = append(args, prog)
	}
	if since > 0 {
		where += " AND slot >= $" + itoa(len(args)+1)
		args = append(args, since)
	}
	if until > 0 {
		where += " AND slot <= $" + itoa(len(args)+1)
		args = append(args, until)
	}

	args = append(args, n)
	sql := `
SELECT signature, slot,
       CASE WHEN err::text = 'null' THEN NULL ELSE err::text END AS err,
       logs, to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') AS created_at
FROM tx_events
WHERE ` + where + `
ORDER BY slot DESC
LIMIT $` + itoa(len(args))

	rows, err := dbStore.Query(ctx, sql, args...)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := make([]EventRow, 0, n)
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.Signature, &e.Slot, &e.Err, &e.Logs, &e.CreatedAt); err != nil {
			http.Error(w, "scan failed", http.StatusInternalServerError)
			return
		}
		list = append(list, e)
	}
	if rows.Err() != nil {
		http.Error(w, "iter failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count": len(list),
		"items": list,
	})
}

// tiny helpers
func intFrom(r *http.Request, k string, d int) int {
	if v := r.URL.Query().Get(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil { return n }
	}
	return d
}
func int64From(r *http.Request, k string, d int64) int64 {
	if v := r.URL.Query().Get(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil { return n }
	}
	return d
}
func itoa(i int) string { return strconv.Itoa(i) }
func clamp(v, lo, hi int) int {
	if v < lo { return lo }
	if v > hi { return hi }
	return v
}
