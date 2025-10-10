
package backfill

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rileyafox/solana-sentinel/internal/parse"
	"github.com/rileyafox/solana-sentinel/internal/rpc"
	"github.com/rileyafox/solana-sentinel/internal/store"
)

type Backfill struct {
	HTTP  *rpc.HTTPClient
	Store *store.Store
}

func New(http *rpc.HTTPClient, st *store.Store) *Backfill {
	return &Backfill{HTTP: http, Store: st}
}

// ScanAccount fetches recent signatures for an address (account or program) and persists tx + events.
func (b *Backfill) ScanAccount(ctx context.Context, pubkey string, limit int) error {
	sigs, err := b.HTTP.GetSignaturesForAddress(ctx, pubkey, limit, "")
	if err != nil {
		return err
	}
	log.Printf("backfill: %d signatures for %s", len(sigs), pubkey)
	for i, s := range sigs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		txres, err := b.HTTP.GetTransaction(ctx, s.Signature)
		if err != nil {
			log.Printf("getTransaction[%d] %s: %v", i, s.Signature, err)
			continue
		}
		txRow, events := parse.FromGetTransaction(s.Signature, txres.Transaction, txres.Meta, txres.Slot, txres.BlockTime)

		if err := b.Store.InsertTransaction(ctx, txRow); err != nil {
			return fmt.Errorf("insert tx %s: %w", txRow.Signature, err)
		}
		if err := b.Store.ReplaceEventsForSignature(ctx, txRow.Signature, events); err != nil {
			return fmt.Errorf("replace events %s: %w", txRow.Signature, err)
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}
