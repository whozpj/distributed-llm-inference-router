.PHONY: proto build test bench

proto:
	mkdir -p gen
	protoc -I proto \
	  --go_out=gen --go_opt=paths=source_relative \
	  --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
	  inference.proto

build: proto
	go build ./...

test: proto
	go test ./... -race -count=1

bench: build
	go run ./cmd/loadgen --seed=42 --scenario=all
