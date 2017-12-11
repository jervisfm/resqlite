#!/usr/bin/env sh

# Start Raft cluster with just 1 node.
# Note(jmuindi): Enabling race detectors slows down startup time to 30s when using sqlite dbs.
# go run -race main.go  -nodes=localhost:50050,

go run main.go  -nodes=localhost:50050
