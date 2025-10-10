package main

import (
	"log"
	"os"
	"sync"

	"github.com/joho/godotenv"
	"github.com/rileyafox/solana-sentinel/internal/api"
	"github.com/rileyafox/solana-sentinel/internal/buildinfo"
	"github.com/rileyafox/solana-sentinel/internal/metrics"
	"github.com/rileyafox/solana-sentinel/internal/stream"
)

func main() {
	_ = godotenv.Load("configs/.env")
	grpcAddr := env("GRPC_ADDR", ":8081")
	restAddr := env("REST_ADDR", ":8080")
	metAddr  := env("METRICS_ADDR", ":9102")
	redisURL := env("REDIS_URL",  "redis://localhost:6379")

	metrics.StartServer(metAddr)

	s := stream.New(redisURL) // includes dedupe & reconnect
	apiSrv := api.NewServer(s, buildinfo.Version())

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); log.Fatal(api.RunGRPC(grpcAddr, apiSrv)) }()
	go func() { defer wg.Done(); log.Fatal(api.RunREST(restAddr, grpcAddr)) }()
	wg.Wait()
}

func env(k, d string) string { if v := os.Getenv(k); v != "" { return v }; return d }
