package server

import (
	"context"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/replica/batcher"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	pb.UnimplementedInferenceServiceServer
	batcher *batcher.Batcher
}

func New(b *batcher.Batcher) *Server { return &Server{batcher: b} }

func (s *Server) Infer(ctx context.Context, req *pb.InferRequest) (*pb.InferResponse, error) {
	ch := make(chan batcher.Result, 1)
	s.batcher.Submit(&batcher.Request{
		ID:           req.RequestId,
		PromptTokens: req.PromptTokens,
		MaxNewTokens: req.MaxNewTokens,
		ResultCh:     ch,
	})
	select {
	case res := <-ch:
		if res.Err != nil {
			return nil, status.Errorf(codes.Internal, "batcher: %v", res.Err)
		}
		return &pb.InferResponse{
			RequestId:     req.RequestId,
			OutputTokens:  res.OutputTokens,
			CacheHit:      res.CacheHit,
			PrefillTokens: res.PrefillTokens,
		}, nil
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func (s *Server) InferStream(req *pb.InferRequest, stream pb.InferenceService_InferStreamServer) error {
	resp, err := s.Infer(stream.Context(), req)
	if err != nil {
		return err
	}
	for i, tok := range resp.OutputTokens {
		if err := stream.Send(&pb.TokenResponse{
			RequestId: req.RequestId,
			Token:     tok,
			Last:      i == len(resp.OutputTokens)-1,
		}); err != nil {
			return err
		}
	}
	return nil
}
