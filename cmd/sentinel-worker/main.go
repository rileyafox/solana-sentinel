
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rileyafox/solana-sentinel/internal/backfill"
	"github.com/rileyafox/solana-sentinel/internal/rpc"
	"github.com/rileyafox/solana-sentinel/internal/store"
)

func main() {
	mode := flag.String("mode", "help", "help|ping|sigs|tx|getacct|logs|backfill")
	addr := flag.String("addr", "", "account or program address (for sigs/logs/backfill)")
	limit := flag.Int("limit", 25, "limit for signatures/backfill")
	sig := flag.String("sig", "", "transaction signature (for tx)")
	httpURL := flag.String("http", getenv("SOLANA_HTTP_URL", "https://api.devnet.solana.com"), "Solana HTTP RPC")
	wsURL := flag.String("ws", getenv("SOLANA_WS_URL", "wss://api.devnet.solana.com"), "Solana WS RPC")
	dsn := flag.String("db", getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5433/sentinel?sslmode=disable"), "Postgres DSN")
	flag.Parse()

	switch *mode {
	case "help":
		fmt.Println("modes:")
		fmt.Println("  ping                     - quick getSlot RPC to verify connectivity")
		fmt.Println("  sigs   -addr <pubkey> [-limit N]")
		fmt.Println("  tx     -sig <signature>")
		fmt.Println("  getacct -addr <pubkey>")
		fmt.Println("  logs   -addr <program_id>  (subscribe logs mentions)")
		fmt.Println("  backfill -addr <pubkey> [-limit N]  (persist tx + events)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch *mode {
	case "backfill":
		if *addr == "" { log.Fatal("-addr required") }
		st, err := store.New(context.Background(), *dsn)
		if err != nil { log.Fatalf("store: %v", err) }
		defer st.Close()
		httpc := rpc.NewHTTPClient(*httpURL)
		b := backfill.New(httpc, st)
		start := time.Now()
		if err := b.ScanAccount(ctx, *addr, *limit); err != nil {
			log.Fatalf("backfill error: %v", err)
		}
		elapsed := time.Since(start)
		log.Printf("backfill done in %s", elapsed)
		recent, _ := st.ListRecentTxs(context.Background(), 5)
		for _, r := range recent {
			log.Printf("tx %s slot=%d fee=%d", r.Signature, r.Slot, r.Fee)
		}
		return

	case "ping":
		h := rpc.NewHTTPClient(*httpURL)
		if err := h.Ping(ctx); err != nil {
			log.Fatalf("ping failed: %v", err)
		}
		log.Println("ping ok")
		return

	case "sigs":
		if *addr == "" { log.Fatal("-addr required") }
		h := rpc.NewHTTPClient(*httpURL)
		sigs, err := h.GetSignaturesForAddress(ctx, *addr, *limit, "")
		if err != nil { log.Fatalf("getSignaturesForAddress: %v", err) }
		for i, s := range sigs {
			fmt.Printf("%2d  %s  slot=%d  err=%v  blockTime=%v\n", i+1, s.Signature, s.Slot, s.Err, s.BlockTime)
		}
		return

	case "tx":
		if *sig == "" { log.Fatal("-sig required") }
		h := rpc.NewHTTPClient(*httpURL)
		tx, err := h.GetTransaction(ctx, *sig)
		if err != nil { log.Fatalf("getTransaction: %v", err) }
		fmt.Printf("slot=%d version=%v blockTime=%v\n", tx.Slot, tx.Version, tx.BlockTime)
		fmt.Printf("meta keys: ")
		for k := range tx.Meta { fmt.Printf("%s ", k) }
		fmt.Println()
		return

	case "getacct":
		if *addr == "" { log.Fatal("-addr required") }
		h := rpc.NewHTTPClient(*httpURL)
		info, err := h.GetAccountInfo(ctx, *addr)
		if err != nil { log.Fatalf("getAccountInfo: %v", err) }
		if info.Value == nil {
			fmt.Println("no account data"); return
		}
		fmt.Printf("slot=%d lamports=%d owner=%s exec=%v rentEpoch=%d\n",
			info.Context.Slot, info.Value.Lamports, info.Value.Owner, info.Value.Executable, info.Value.RentEpoch)
		return

	case "logs":
		if *addr == "" { log.Fatal("-addr (program id to 'mentions') required") }
		ws := rpc.NewWSClient(*wsURL)
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel2()
		ch, err := ws.SubscribeLogs(ctx2, map[string]any{"mentions": []string{*addr}})
		if err != nil { log.Fatalf("subscribe logs: %v", err) }
		log.Println("listening (Ctrl+C to stop)...")
		for msg := range ch {
			fmt.Printf("slot=%d sig=%s logs=%d\n", msg.Params.Result.Context.Slot, msg.Params.Result.Value.Signature, len(msg.Params.Result.Value.Logs))
		}
		return

	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" { return v }
	return def
}
