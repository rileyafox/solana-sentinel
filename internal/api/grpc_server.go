package api

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	tx "github.com/rileyafox/solana-sentinel/api/gen/txrelay/v1"
	"github.com/rileyafox/solana-sentinel/internal/stream"
)

type Server struct {
	tx.UnimplementedSentinelServer
	streamer *stream.Streamer
	version  string
}

func NewServer(s *stream.Streamer, version string) *Server {
	return &Server{streamer: s, version: version}
}

func (s *Server) Health(ctx context.Context, _ *tx.HealthRequest) (*tx.HealthResponse, error) {
	return &tx.HealthResponse{Status: "ok", Version: s.version}, nil
}

// Stream delegates to the streamer, which already handles
// dedupe, metrics, and marshaling to tx.Event.
func (s *Server) Stream(req *tx.StreamRequest, srv tx.Sentinel_StreamServer) error {
	// Pass the stream's context so cancellation closes the stream cleanly.
	return s.streamer.StreamToClient(srv.Context(), req, srv)
}

func RunGRPC(addr string, s *Server) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
	 return err
	}

	gs := grpc.NewServer()
	tx.RegisterSentinelServer(gs, s)

	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)

	return gs.Serve(lis)
}
