package api

import (
	"context"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	tx "github.com/rileyafox/solana-sentinel/api/gen/txrelay/v1"
	"google.golang.org/grpc"
)

func RunREST(restAddr, grpcAddr string) error {
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()} // TODO: mTLS in prod phase
	if err := tx.RegisterSentinelHandlerFromEndpoint(context.Background(), mux, grpcAddr, opts); err != nil {
		return err
	}
	srv := &http.Server{Addr: restAddr, Handler: mux}
	return srv.ListenAndServe()
}
