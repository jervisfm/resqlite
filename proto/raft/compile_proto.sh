#!/usr/bin/env sh

# Compile the raft proto to generate Go Lang RPC stub code.
protoc raft.proto --go_out=plugins=grpc:.