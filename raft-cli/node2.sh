#!/usr/bin/env sh

# Start node 2 of cluster.
#go run -race main.go  -nodes=localhost:50052,localhost:50051,localhost:50053
go run main.go  -nodes=localhost:50052,localhost:50051,localhost:50053
