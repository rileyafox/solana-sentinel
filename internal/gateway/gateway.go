package gateway

import (
	"context"
	"log"
	"net/http"

	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	tx "github.com/rileyafox/solana-sentinel/api/gen/txrelay/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewHTTPMux(ctx context.Context, target string) http.Handler {
	// grpc-gateway mux
	gwmux := grpcgw.NewServeMux()

	// Dial options: insecure for local dev, no WithBlock (avoid startup race)
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if err := tx.RegisterSentinelHandlerFromEndpoint(
		context.Background(), gwmux, target, dialOpts,
	); err != nil {
		log.Fatalf("gateway register error: %v (is gRPC running at %s?)", err, target)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", withCORS(gwmux))
	return mux
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
