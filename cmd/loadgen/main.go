package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/loadgen"
	"distributed-llm-inference-router/metrics"
	"distributed-llm-inference-router/replica/batcher"
	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
	replicaserver "distributed-llm-inference-router/replica/server"
	"distributed-llm-inference-router/router"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var scenarios = []struct {
	name            string
	overlapFraction float64
	targetRPS       float64
}{
	{"cache-heavy", 0.80, 50},
	{"cache-cold", 0.05, 50},
	{"bursty", 0.50, 200},
}

var policies = []struct {
	name    string
	factory func(replicas []*router.ReplicaClient) router.Policy
}{
	{"roundrobin", func(_ []*router.ReplicaClient) router.Policy {
		return router.NewRoundRobin()
	}},
	{"lor", func(_ []*router.ReplicaClient) router.Policy {
		return router.NewLeastOutstanding()
	}},
	{"prefixaware", func(_ []*router.ReplicaClient) router.Policy {
		pm := router.NewPrefixMap(4096, 5*time.Minute)
		return router.NewPrefixAware(router.PrefixAwareConfig{
			PrefixLen:        16,
			LocalityWeight:   1.0,
			HotspotThreshold: 20,
		}, pm, router.NewLeastOutstanding())
	}},
}

func startReplica(tickInterval time.Duration) (string, func()) {
	m := model.New(model.Config{
		Base:            5 * time.Millisecond,
		PrefillPerToken: 2 * time.Millisecond,
		DecodePerToken:  1 * time.Millisecond,
	})
	kv := cache.New(1024, 5*time.Minute)
	b := batcher.New(batcher.Config{
		MaxBatch:     8,
		TokenBudget:  4096,
		TickInterval: tickInterval,
		PrefixLen:    16,
	}, m, kv)
	b.Start()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, replicaserver.New(b))
	go gs.Serve(lis)
	return lis.Addr().String(), func() { gs.Stop(); b.Stop() }
}

func main() {
	seed := flag.Int64("seed", 42, "random seed")
	scenario := flag.String("scenario", "all", "scenario name or 'all'")
	nReplicas := flag.Int("replicas", 3, "number of replicas")
	totalReqs := flag.Int("requests", 200, "requests per run")
	flag.Parse()

	fmt.Printf("%-14s  %-12s  %8s  %8s  %8s  %8s  %8s\n",
		"scenario", "policy", "count", "p50", "p90", "p99", "cache%")
	fmt.Println("----------------------------------------------------------------------")

	for _, sc := range scenarios {
		if *scenario != "all" && sc.name != *scenario {
			continue
		}
		for _, pol := range policies {
			addrs := make([]string, *nReplicas)
			stops := make([]func(), *nReplicas)
			for i := range addrs {
				addrs[i], stops[i] = startReplica(5 * time.Millisecond)
			}
			replicas := make([]*router.ReplicaClient, *nReplicas)
			for i, addr := range addrs {
				r, err := router.NewReplicaClient(addr, addr)
				if err != nil {
					log.Fatalf("dial: %v", err)
				}
				replicas[i] = r
			}

			p := pol.factory(replicas)
			rt := router.New(router.Config{QueueDepth: 512, RPCTimeout: 10 * time.Second}, replicas, p)
			rt.Start(8)

			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				log.Fatalf("listen: %v", err)
			}
			gs := grpc.NewServer()
			pb.RegisterInferenceServiceServer(gs, rt)
			go gs.Serve(lis)

			conn, err := grpc.Dial(lis.Addr().String(), //nolint:staticcheck
				grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Fatalf("dial router: %v", err)
			}

			rec := metrics.New()
			lg := loadgen.New(loadgen.Config{
				Seed:            *seed,
				TargetRPS:       sc.targetRPS,
				TotalRequests:   *totalReqs,
				OverlapFraction: sc.overlapFraction,
				SharedPrefixLen: 16,
				PromptLenMin:    20,
				PromptLenMax:    64,
				MaxNewTokens:    10,
			}, pb.NewInferenceServiceClient(conn), rec)

			lg.Run(context.Background())

			s := rec.Summary()
			fmt.Printf("%-14s  %-12s  %8d  %8s  %8s  %8s  %7.1f%%\n",
				sc.name, pol.name, s.Count,
				s.P50.Round(time.Millisecond),
				s.P90.Round(time.Millisecond),
				s.P99.Round(time.Millisecond),
				s.CacheHitRate*100,
			)

			conn.Close()
			gs.Stop()
			rt.Stop()
			for _, r := range replicas {
				r.Close()
			}
			for _, stop := range stops {
				stop()
			}
		}
	}
}
