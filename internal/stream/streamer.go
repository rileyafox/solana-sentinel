package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tx "github.com/rileyafox/solana-sentinel/api/gen/txrelay/v1"
	"github.com/rileyafox/solana-sentinel/internal/dedupe"
	"github.com/rileyafox/solana-sentinel/internal/metrics"
)

type Event struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Slot    string `json:"slot"`
	Account string `json:"account"`
	Program string `json:"program"`
	Payload []byte `json:"payload"`
	TSms    int64  `json:"ts_ms"`
}

type Streamer struct {
	Dedupe *dedupe.RedisDedupe
}

func New(redisURL string) *Streamer { return &Streamer{Dedupe: dedupe.New(redisURL)} }

// Subscribe produces already-deduped events and updates metrics,
// so metrics move even if no client is connected.
func (s *Streamer) Subscribe(ctx context.Context, req *tx.StreamRequest) <-chan Event {
	out := make(chan Event, 1024)

	// DEMO producer: remove once real ingestion is wired
	go func() {
		defer close(out)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				i++
				ev := Event{
					ID:      fmt.Sprintf("demo-%d", i),
					Kind:    "tx",
					Slot:    "0",
					Account: "acc1",
					Program: "prog1",
					Payload: []byte(`{"demo":true}`),
					TSms:    time.Now().UnixMilli(),
				}

				// Dedup + metrics here
				if !s.Dedupe.TryEmit(ev.ID, 5*time.Second) {
					metrics.DedupedEvents.WithLabelValues(ev.Kind).Inc()
					continue
				}
				metrics.EmittedEvents.WithLabelValues(ev.Kind).Inc()

				// Emit to subscribers
				out <- ev
			}
		}
	}()

	return out
}

// Streams already-filtered events to the client.
func (s *Streamer) StreamToClient(ctx context.Context, req *tx.StreamRequest, stream tx.Sentinel_StreamServer) error {
	ch := s.Subscribe(ctx, req)
	for ev := range ch {
		data, _ := json.Marshal(ev)
		if err := stream.Send(&tx.Event{
			Id:      ev.ID,
			Kind:    ev.Kind,
			Slot:    ev.Slot,
			Account: ev.Account,
			Program: ev.Program,
			Payload: data,
			TsMs:    ev.TSms,
		}); err != nil {
			metrics.StreamErrors.Inc()
			return err
		}
	}
	return nil
}
