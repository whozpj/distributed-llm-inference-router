package main

import (
	"flag"
	"log"
	"net"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/replica/batcher"
	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
	"distributed-llm-inference-router/replica/server"

	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", ":50051", "listen address")
	maxBatch := flag.Int("max-batch", 8, "max concurrent sequences")
	tokenBudget := flag.Int("token-budget", 4096, "max tokens in flight")
	flag.Parse()

	m := model.New(model.Config{
		Base:            5 * time.Millisecond,
		PrefillPerToken: 2 * time.Millisecond,
		DecodePerToken:  1 * time.Millisecond,
	})
	kv := cache.New(1024, 5*time.Minute)
	b := batcher.New(batcher.Config{
		MaxBatch:     *maxBatch,
		TokenBudget:  *tokenBudget,
		TickInterval: 10 * time.Millisecond,
		PrefixLen:    16,
	}, m, kv)
	b.Start()
	defer b.Stop()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterInferenceServiceServer(gs, server.New(b))
	log.Printf("replica listening on %s", *addr)
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
