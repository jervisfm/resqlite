#!/usr/bin/env sh

# Start node 1 of cluster.
go run -race main.go  -nodes=localhost:50051,localhost:50052,localhost:50053
