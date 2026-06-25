package router

import (
	"context"
	"time"

	pb "distributed-llm-inference-router/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Config struct {
	QueueDepth int
	RPCTimeout time.Duration
}

type queued struct {
	ctx    context.Context
	req    *pb.InferRequest
	respCh chan queuedResult
}

type queuedResult struct {
	resp *pb.InferResponse
	err  error
}

type Router struct {
	pb.UnimplementedInferenceServiceServer
	cfg      Config
	replicas []*ReplicaClient
	policy   Policy
	queue    chan *queued
	stop     chan struct{}
}

func New(cfg Config, replicas []*ReplicaClient, policy Policy) *Router {
	return &Router{
		cfg:      cfg,
		replicas: replicas,
		policy:   policy,
		queue:    make(chan *queued, cfg.QueueDepth),
		stop:     make(chan struct{}),
	}
}

func (r *Router) Start(workers int) {
	for i := 0; i < workers; i++ {
		go r.worker()
	}
}

func (r *Router) Stop() { close(r.stop) }

func (r *Router) Infer(ctx context.Context, req *pb.InferRequest) (*pb.InferResponse, error) {
	ch := make(chan queuedResult, 1)
	q := &queued{ctx: ctx, req: req, respCh: ch}
	select {
	case r.queue <- q:
	default:
		return nil, status.Error(codes.ResourceExhausted, "router queue full")
	}
	select {
	case res := <-ch:
		return res.resp, res.err
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func (r *Router) worker() {
	for {
		select {
		case <-r.stop:
			return
		case q := <-r.queue:
			replica := r.policy.Pick(&Request{
				ID:           q.req.RequestId,
				PromptTokens: q.req.PromptTokens,
				MaxNewTokens: q.req.MaxNewTokens,
			}, r.replicas)
			if replica == nil {
				q.respCh <- queuedResult{err: status.Error(codes.Unavailable, "no healthy replicas")}
				continue
			}
			ctx := q.ctx
			if r.cfg.RPCTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, r.cfg.RPCTimeout)
				defer cancel()
			}
			resp, err := replica.Infer(ctx, q.req)
			q.respCh <- queuedResult{resp: resp, err: err}
		}
	}
}
