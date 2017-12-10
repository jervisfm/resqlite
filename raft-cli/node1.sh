#!/usr/bin/env sh

# Start node 1 of cluster.
# Note(jmuindi): Enabling race detectors slows down startup time to 30s when using sqlite dbs.
# go run -race main.go  -nodes=localhost:50051,localhost:50052,localhost:50053

go run main.go  -nodes=localhost:50051,localhost:50052,localhost:50053
