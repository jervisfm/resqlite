#!/usr/bin/env sh

# Start up node 3 of the cluster.
go run -race main.go  -nodes=localhost:50053,localhost:50052,localhost:50051
