package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type HTTPClient struct {
	BaseURL    string
	HTTP       *http.Client
	MaxRetries int
}

func NewHTTPClient(base string) *HTTPClient {
	return &HTTPClient{
		BaseURL:    base,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
		MaxRetries: 3,
	}
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse[T any] struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Result  T         `json:"result"`
	Error   *rpcError `json:"error,omitempty"`
}

// Standalone generic function (allowed): makes the JSON-RPC call and decodes Result into T.
func rpcDo[T any](ctx context.Context, c *HTTPClient, method string, params any) (T, error) {
	var zero T

	body, _ := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	})

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
		if err != nil {
			return zero, err
		}
		req.Header.Set("Content-Type", "application/json")

		res, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
		} else {
			defer res.Body.Close()
			if res.StatusCode >= 500 {
				lastErr = fmt.Errorf("rpc server %s", res.Status)
			} else {
				var wrapper rpcResponse[T]
				if err := json.NewDecoder(res.Body).Decode(&wrapper); err != nil {
					return zero, err
				}
				if wrapper.Error != nil {
					return zero, errors.New(wrapper.Error.Message)
				}
				return wrapper.Result, nil
			}
		}
		// simple backoff
		select {
		case <-time.After(time.Duration(attempt+1) * 400 * time.Millisecond):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	return zero, lastErr
}

type SignatureInfo struct {
	Signature string  `json:"signature"`
	Slot      uint64  `json:"slot"`
	Err       any     `json:"err"`
	BlockTime *int64  `json:"blockTime"`
	Memo      *string `json:"memo"`
}

func (c *HTTPClient) GetSignaturesForAddress(ctx context.Context, address string, limit int, before string) ([]SignatureInfo, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	type opts struct {
		Limit  int    `json:"limit,omitempty"`
		Before string `json:"before,omitempty"`
	}
	params := []any{address, opts{Limit: limit, Before: before}}
	return rpcDo[[]SignatureInfo](ctx, c, "getSignaturesForAddress", params)
}

type GetTransactionResult struct {
	Slot        uint64                 `json:"slot"`
	BlockTime   *int64                 `json:"blockTime"`
	Meta        map[string]any         `json:"meta"`
	Transaction map[string]any         `json:"transaction"`
	Version     any                    `json:"version"`
}

func (c *HTTPClient) GetTransaction(ctx context.Context, signature string) (*GetTransactionResult, error) {
	params := []any{
		signature,
		map[string]any{
			"encoding":                       "jsonParsed",
			"maxSupportedTransactionVersion": 0,
			"commitment":                     "confirmed",
		},
	}
	return rpcDo[*GetTransactionResult](ctx, c, "getTransaction", params)
}

type AccountInfoResp struct {
	Context struct {
		Slot uint64 `json:"slot"`
	} `json:"context"`
	Value *struct {
		Lamports   uint64 `json:"lamports"`
		Owner      string `json:"owner"`
		Executable bool   `json:"executable"`
		RentEpoch  uint64 `json:"rentEpoch"`
		Data       any    `json:"data"`
	} `json:"value"`
}

func (c *HTTPClient) GetAccountInfo(ctx context.Context, address string) (*AccountInfoResp, error) {
	params := []any{
		address,
		map[string]any{
			"encoding":   "jsonParsed",
			"commitment": "confirmed",
		},
	}
	return rpcDo[*AccountInfoResp](ctx, c, "getAccountInfo", params)
}

// Quick liveness check
func (c *HTTPClient) Ping(ctx context.Context) error {
	_, err := rpcDo[uint64](ctx, c, "getSlot", []any{})
	return err
}
