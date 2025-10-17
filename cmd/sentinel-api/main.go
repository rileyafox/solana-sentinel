package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	tx "github.com/rileyafox/solana-sentinel/api/gen/txrelay/v1"
	apihttp "github.com/rileyafox/solana-sentinel/internal/api"
	"github.com/rileyafox/solana-sentinel/internal/gateway"
	"github.com/rileyafox/solana-sentinel/internal/observability"
	"github.com/rileyafox/solana-sentinel/internal/store"
	"github.com/rileyafox/solana-sentinel/internal/stream"
	"github.com/rileyafox/solana-sentinel/internal/worker"

	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	grpcAddr := getenv("GRPC_ADDR", ":8081")
	restAddr := getenv("REST_ADDR", ":8080")
	_ = getenv("METRICS_ADDR", ":9102")

	redisURL := getenv("REDIS_URL", "redis://redis:6379/0")
	dsn := getenv("DATABASE_URL", "postgres://postgres:postgres@db:5432/sentinel?sslmode=disable")

	// ---- Observability / shutdown ----
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	shutdown := observability.Init(ctx)
	defer shutdown()

	// ---- DB store for HTTP handlers ----
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	if err := st.EnsureSchema(ctx); err != nil {
		log.Fatalf("schema: %v", err)
	}
	apihttp.SetStore(st) // make store available to LatestEventsHandler

	// ---- Background worker: Redis -> Postgres ----
	go func() {
		if err := worker.RunRedisToPostgres(context.Background()); err != nil {
			log.Printf("worker exited: %v", err)
		}
	}()

	// ---- gRPC server + health ----
	grpcSrv := grpc.NewServer()

	streamer := stream.New(redisURL)
	svc := apihttp.NewServer(streamer, "dev")
	tx.RegisterSentinelServer(grpcSrv, svc)

	hs := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen gRPC %s: %v", grpcAddr, err)
	}
	go func() {
		log.Printf("gRPC listening on %s", grpcAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	// ---- REST: our routes FIRST, then grpc-gateway ----
	gw := gateway.NewHTTPMux(ctx, trimHostPort(restDialTarget(grpcAddr)))

	root := http.NewServeMux()
	root.HandleFunc("/v1/events/latest", apihttp.LatestEventsHandler) // custom REST endpoint
	// optional: simple health for REST plane
	root.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","plane":"rest"}`))
	})
	// everything else → grpc-gateway (OpenAPI/REST ↔ gRPC)
	root.Handle("/", gw)

	httpSrv := &http.Server{
		Addr:              restAddr,
		Handler:           root, // IMPORTANT: use root, not gw directly
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("REST gateway on %s (→ %s)", restAddr, grpcAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	grpcSrv.GracefulStop()
	log.Println("bye")
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func restDialTarget(grpcAddr string) string {
	if len(grpcAddr) > 0 && grpcAddr[0] == ':' {
		return "localhost" + grpcAddr
	}
	return grpcAddr
}

func trimHostPort(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "localhost" + addr
	}
	return addr
}
