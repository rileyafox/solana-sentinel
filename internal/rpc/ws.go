package rpc

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	URL        string
	dialer     *websocket.Dialer
	MaxBackoff time.Duration
}

func NewWSClient(url string) *WSClient {
	return &WSClient{
		URL:        url,
		dialer:     &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
		MaxBackoff: 20 * time.Second,
	}
}

// LogMsg is an envelope for logsSubscribe notifications.
type LogMsg struct {
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

func (c *WSClient) SubscribeLogs(ctx context.Context, filter any) (<-chan LogMsg, error) {
	out := make(chan LogMsg, 256)

	go func() {
		defer close(out)

		backoff := 500 * time.Millisecond
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			conn, _, err := c.dialer.Dial(c.URL, nil)
			if err != nil {
				log.Printf("ws: dial error: %v", err)
				backoff = minDuration(backoff*2, c.MaxBackoff)
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return
				}
			}
			backoff = 500 * time.Millisecond
			log.Printf("ws: connected to %s", c.URL)

			// Subscribe
			subReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "logsSubscribe",
				"params":  []any{filter, "confirmed"},
			}
			if err := conn.WriteJSON(subReq); err != nil {
				log.Printf("ws: write subscribe error: %v", err)
				_ = conn.Close()
				continue
			}

			readDone := make(chan struct{})
			go func() {
				defer close(readDone)
				for {
					_, data, err := conn.ReadMessage()
					if err != nil {
						log.Printf("ws: read error: %v", err)
						return
					}
					if !json.Valid(data) {
						continue
					}
					var maybe map[string]any
					if err := json.Unmarshal(data, &maybe); err != nil {
						continue
					}
					// notifications carry "params"
					if maybe["params"] != nil {
						var msg LogMsg
						if err := json.Unmarshal(data, &msg); err == nil {
							select {
							case out <- msg:
							default:
								// drop on backpressure; count later
							}
						}
					}
				}
			}()

			select {
			case <-ctx.Done():
				_ = conn.Close()
				return
			case <-readDone:
				_ = conn.Close()
				// loop and reconnect
			}
		}
	}()

	return out, nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
