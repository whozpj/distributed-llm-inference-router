package router_test

import (
	"context"
	"net"
	"testing"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/replica/batcher"
	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
	replicaserver "distributed-llm-inference-router/replica/server"
	"distributed-llm-inference-router/router"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const testBufSize = 1 << 20

func startTestReplica(t *testing.T) string {
	t.Helper()
	m := model.New(model.Config{PrefillPerToken: time.Millisecond, DecodePerToken: time.Millisecond})
	kv := cache.New(100, time.Minute)
	b := batcher.New(batcher.Config{MaxBatch: 4, TokenBudget: 10_000, TickInterval: time.Millisecond, PrefixLen: 8}, m, kv)
	b.Start()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, replicaserver.New(b))
	go gs.Serve(lis)
	t.Cleanup(func() { gs.Stop(); b.Stop() })
	return lis.Addr().String()
}

func newTestRouter(t *testing.T, addrs []string) pb.InferenceServiceClient {
	t.Helper()
	replicas := make([]*router.ReplicaClient, len(addrs))
	for i, addr := range addrs {
		r, err := router.NewReplicaClient(addr, addr)
		if err != nil {
			t.Fatalf("dial replica: %v", err)
		}
		replicas[i] = r
		t.Cleanup(func() { r.Close() })
	}

	rt := router.New(router.Config{QueueDepth: 64, RPCTimeout: 5 * time.Second},
		replicas, router.NewRoundRobin())
	rt.Start(4)

	lis := bufconn.Listen(testBufSize)
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, rt)
	go gs.Serve(lis)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial router: %v", err)
	}
	t.Cleanup(func() { conn.Close(); gs.Stop(); rt.Stop() })
	return pb.NewInferenceServiceClient(conn)
}

func TestRouterForwardsToReplica(t *testing.T) {
	addr := startTestReplica(t)
	client := newTestRouter(t, []string{addr})

	resp, err := client.Infer(context.Background(), &pb.InferRequest{
		RequestId:    "smoke-1",
		PromptTokens: []int32{1, 2, 3},
		MaxNewTokens: 3,
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if resp.RequestId != "smoke-1" {
		t.Fatalf("got request_id %q", resp.RequestId)
	}
}

func TestRouterQueueFullRejectsRequests(t *testing.T) {
	addr := startTestReplica(t)
	replicas := make([]*router.ReplicaClient, 1)
	r, _ := router.NewReplicaClient(addr, addr)
	replicas[0] = r
	t.Cleanup(func() { r.Close() })

	rt := router.New(router.Config{QueueDepth: 1, RPCTimeout: 5 * time.Second}, replicas, router.NewRoundRobin())
	rt.Start(1)
	defer rt.Stop()

	lis := bufconn.Listen(testBufSize)
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, rt)
	go gs.Serve(lis)
	defer gs.Stop()

	conn, _ := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	defer conn.Close()
	client := pb.NewInferenceServiceClient(conn)

	errs := 0
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, err := client.Infer(ctx, &pb.InferRequest{RequestId: "flood", PromptTokens: []int32{1}, MaxNewTokens: 50})
		cancel()
		if err != nil {
			errs++
		}
	}
	if errs == 0 {
		t.Fatal("expected some requests to be rejected under flood")
	}
}
