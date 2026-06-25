package main

import (
	"flag"
	"log"
	"net"
	"strings"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/router"

	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", ":50050", "listen address")
	replicaAddrs := flag.String("replicas", ":50051,:50052", "comma-separated replica addresses")
	policy := flag.String("policy", "roundrobin", "roundrobin | lor | prefixaware")
	metricsAddr := flag.String("metrics-addr", ":9100", "Prometheus metrics address; empty to disable")
	flag.Parse()

	addrs := strings.Split(*replicaAddrs, ",")
	replicas := make([]*router.ReplicaClient, len(addrs))
	for i, a := range addrs {
		r, err := router.NewReplicaClient(a, strings.TrimSpace(a))
		if err != nil {
			log.Fatalf("dial replica %s: %v", a, err)
		}
		replicas[i] = r
		defer r.Close()
	}

	var p router.Policy
	switch *policy {
	case "roundrobin":
		p = router.NewRoundRobin()
	case "lor":
		p = router.NewLeastOutstanding()
	case "prefixaware":
		pm := router.NewPrefixMap(4096, 5*time.Minute)
		p = router.NewPrefixAware(router.PrefixAwareConfig{
			PrefixLen:        16,
			LocalityWeight:   1.0,
			HotspotThreshold: 20,
		}, pm, router.NewLeastOutstanding())
	default:
		log.Fatalf("unknown policy %q", *policy)
	}

	rt := router.New(router.Config{
		QueueDepth:  256,
		RPCTimeout:  30 * time.Second,
		PolicyName:  *policy,
		MetricsAddr: *metricsAddr,
	}, replicas, p)
	rt.Start(8)
	defer rt.Stop()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, rt)
	log.Printf("router listening on %s (policy=%s, metrics=%s)", *addr, *policy, *metricsAddr)
	gs.Serve(lis)
}
