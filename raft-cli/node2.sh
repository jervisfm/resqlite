#!/usr/bin/env sh

# Start node 2 of cluster.
go run main.go  -nodes=localhost:50052,localhost:50051,localhost:50053
