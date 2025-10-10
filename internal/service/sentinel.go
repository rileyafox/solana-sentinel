
package service

import (
	"context"

	apiv1 "github.com/rileyafox/solana-sentinel/api/v1"
)

type SentinelService struct {
	apiv1.UnimplementedSentinelServiceServer
}

// GetTransactions — stub implementation.
func (s *SentinelService) GetTransactions(ctx context.Context, req *apiv1.GetTxsRequest) (*apiv1.GetTxsResponse, error) {
	return &apiv1.GetTxsResponse{}, nil
}

// StreamEvents — stub implementation.
func (s *SentinelService) StreamEvents(req *apiv1.StreamEventsRequest, srv apiv1.SentinelService_StreamEventsServer) error {
	return nil
}

func NewSentinelService() *SentinelService { return &SentinelService{} }
