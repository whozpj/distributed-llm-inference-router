package router

import (
	"context"
	"sync/atomic"

	pb "distributed-llm-inference-router/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ReplicaClient struct {
	id          string
	index       int
	addr        string
	conn        *grpc.ClientConn
	stub        pb.InferenceServiceClient
	outstanding atomic.Int64
	healthy     atomic.Bool
}

func NewReplicaClient(id, addr string) (*ReplicaClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck
	if err != nil {
		return nil, err
	}
	r := &ReplicaClient{id: id, addr: addr, conn: conn, stub: pb.NewInferenceServiceClient(conn)}
	r.healthy.Store(true)
	return r, nil
}

func NewTestReplicaClient(index int) *ReplicaClient {
	r := &ReplicaClient{index: index}
	r.healthy.Store(true)
	return r
}

func (r *ReplicaClient) Index() int        { return r.index }
func (r *ReplicaClient) ID() string        { return r.id }
func (r *ReplicaClient) Outstanding() int64 { return r.outstanding.Load() }
func (r *ReplicaClient) IsHealthy() bool   { return r.healthy.Load() }
func (r *ReplicaClient) SetHealthy(v bool) { r.healthy.Store(v) }

func (r *ReplicaClient) Infer(ctx context.Context, req *pb.InferRequest) (*pb.InferResponse, error) {
	r.outstanding.Add(1)
	defer r.outstanding.Add(-1)
	return r.stub.Infer(ctx, req)
}

func (r *ReplicaClient) AddOutstanding(n int64) { r.outstanding.Add(n) }

func (r *ReplicaClient) Close() error {
	if r.conn == nil {
		return nil
	}
	return r.conn.Close()
}
