package main

import (
	"context"
	"log"
	"net"
	"net/http"

	apiv1 "github.com/rileyafox/solana-sentinel/api/v1"
	"github.com/rileyafox/solana-sentinel/internal/gateway"
	"github.com/rileyafox/solana-sentinel/internal/observability"
	"github.com/rileyafox/solana-sentinel/internal/service"

	"google.golang.org/grpc"
	health "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	ctx := context.Background()
	shutdown := observability.Init(ctx)
	defer shutdown()

	grpcSrv := grpc.NewServer()
	svc := service.NewSentinelService()
	apiv1.RegisterSentinelServiceServer(grpcSrv, svc)

	hs := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Start gRPC first
	lis, err := net.Listen("tcp", ":8080")
	if err != nil { log.Fatal(err) }
	go func() {
		log.Println("gRPC listening on :8080")
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	// Then start the REST gateway, pointing at localhost:8080
	mux := gateway.NewHTTPMux(ctx, "localhost:8080")
	log.Println("REST gateway on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}
