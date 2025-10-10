
package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	// "strconv"
	"time"

	"github.com/rileyafox/solana-sentinel/internal/store"
)

// FromGetTransaction normalizes a getTransaction JSON into TxRow + EventRows (basic transfer + program_log).
func FromGetTransaction(signature string, tx map[string]any, meta map[string]any, slot uint64, blockTime *int64) (store.TxRow, []store.EventRow) {
	var fee int64 = 0
	if v, ok := meta["fee"].(float64); ok {
		fee = int64(v)
	}
	var bt *time.Time
	if blockTime != nil {
		t := time.Unix(*blockTime, 0).UTC()
		bt = &t
	}

	// Marshal raw JSON bodies for audit
	raw := mustJSON(map[string]any{"slot": slot, "blockTime": blockTime, "meta": meta, "transaction": tx})

	var errJSON []byte
	if e, ok := meta["err"]; ok && e != nil {
		errJSON = mustJSON(e)
	} else {
		errJSON = []byte("null")
	}

	txRow := store.TxRow{
		Signature: signature,
		Slot:      int64(slot),
		BlockTime: bt,
		Fee:       fee,
		ErrJSON:   errJSON,
		RawJSON:   raw,
	}

	// Events:
	evs := make([]store.EventRow, 0, 4)
	occur := time.Now().UTC()
	if bt != nil {
		occur = *bt
	}

	// 1) Try to extract system "transfer" from jsonParsed
	// transaction.message.instructions[*].parsed.type == "transfer"
	if msg, ok := tx["message"].(map[string]any); ok {
		if insts, ok := msg["instructions"].([]any); ok {
			for _, it := range insts {
				im, _ := it.(map[string]any)
				parsed, _ := im["parsed"].(map[string]any)
				if parsed == nil { continue }
				if t, _ := parsed["type"].(string); t == "transfer" {
					info, _ := parsed["info"].(map[string]any)
					src, _ := toString(info["source"])
					dst, _ := toString(info["destination"])
					lamportsStr := toNumericString(info["lamports"])
					raw := mustJSON(map[string]any{"source": src, "destination": dst, "lamports": info["lamports"]})
					// store amount as lamports numeric string
					evs = append(evs, store.EventRow{
						Kind:       "transfer",
						Signature:  signature,
						Slot:       int64(slot),
						Account:    ptrOrNil(dst),
						Program:    ptrOrNil("11111111111111111111111111111111"),
						Amount:     ptrOrNil(lamportsStr),
						Mint:       nil,
						RawJSON:    raw,
						OccurredAt: occur,
					})
				}
			}
		}
	}

	// 2) Also emit a generic program_log if logs exist
	if logs, ok := meta["logMessages"].([]any); ok && len(logs) > 0 {
		raw := mustJSON(map[string]any{"logs": logs})
		evs = append(evs, store.EventRow{
			Kind:       "program_log",
			Signature:  signature,
			Slot:       int64(slot),
			Account:    nil,
			Program:    nil,
			Amount:     nil,
			Mint:       nil,
			RawJSON:    raw,
			OccurredAt: occur,
		})
	}

	return txRow, evs
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

func toString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case nil:
		return "", false
	default:
		b, _ := json.Marshal(t)
		return string(bytes.Trim(b, `"`)), true
	}
}

func toNumericString(v any) string {
	switch t := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", t)
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(bytes.Trim(b, `"`))
	}
}

func ptrOrNil(s string) *string {
	if s == "" { return nil }
	return &s
}
