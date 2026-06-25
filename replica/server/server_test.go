package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/replica/batcher"
	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
	"distributed-llm-inference-router/replica/server"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1 << 20

func newTestClient(t *testing.T) pb.InferenceServiceClient {
	t.Helper()
	m := model.New(model.Config{PrefillPerToken: time.Millisecond, DecodePerToken: time.Millisecond})
	kv := cache.New(100, time.Minute)
	b := batcher.New(batcher.Config{MaxBatch: 4, TokenBudget: 10_000, TickInterval: time.Millisecond, PrefixLen: 8}, m, kv)
	b.Start()

	srv := server.New(b)
	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, srv)
	go gs.Serve(lis)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close(); gs.Stop(); b.Stop() })
	return pb.NewInferenceServiceClient(conn)
}

func TestInferRoundTrip(t *testing.T) {
	client := newTestClient(t)
	resp, err := client.Infer(context.Background(), &pb.InferRequest{
		RequestId:    "test-1",
		PromptTokens: []int32{1, 2, 3, 4, 5},
		MaxNewTokens: 4,
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if resp.RequestId != "test-1" {
		t.Fatalf("got request_id %q, want test-1", resp.RequestId)
	}
	if len(resp.OutputTokens) != 4 {
		t.Fatalf("got %d output tokens, want 4", len(resp.OutputTokens))
	}
}
