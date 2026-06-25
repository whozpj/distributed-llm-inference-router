package router

import (
	"context"
	"net/http"
	"time"

	pb "distributed-llm-inference-router/gen"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Config struct {
	QueueDepth  int
	RPCTimeout  time.Duration
	PolicyName  string // label used in Prometheus metrics
	MetricsAddr string // e.g. ":9100"; empty disables metrics HTTP server
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
	obs      *Observer
	srv      *http.Server
}

func New(cfg Config, replicas []*ReplicaClient, policy Policy) *Router {
	var obs *Observer
	if cfg.PolicyName != "" {
		reg := prometheus.NewRegistry()
		obs = NewObserver(reg, cfg.PolicyName)
		if cfg.MetricsAddr != "" {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
			srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux}
			go srv.ListenAndServe() //nolint:errcheck
			return &Router{
				cfg:      cfg,
				replicas: replicas,
				policy:   policy,
				queue:    make(chan *queued, cfg.QueueDepth),
				stop:     make(chan struct{}),
				obs:      obs,
				srv:      srv,
			}
		}
	}
	return &Router{
		cfg:      cfg,
		replicas: replicas,
		policy:   policy,
		queue:    make(chan *queued, cfg.QueueDepth),
		stop:     make(chan struct{}),
		obs:      obs,
	}
}

func (r *Router) Start(workers int) {
	for i := 0; i < workers; i++ {
		go r.worker()
	}
}

func (r *Router) Stop() {
	close(r.stop)
	if r.srv != nil {
		r.srv.Shutdown(context.Background()) //nolint:errcheck
	}
}

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
			var cancel context.CancelFunc
			if r.cfg.RPCTimeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, r.cfg.RPCTimeout)
			}
			start := time.Now()
			resp, err := replica.Infer(ctx, q.req)
			if cancel != nil {
				cancel()
			}
			if r.obs != nil {
				cacheHit := err == nil && resp.CacheHit
				r.obs.Observe(time.Since(start), cacheHit)
			}
			q.respCh <- queuedResult{resp: resp, err: err}
		}
	}
}
