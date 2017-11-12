#!/usr/bin/env sh

# Compile the resqlite proto to generate Go Lang RPC stub code.
protoc resqlite.proto --go_out=plugins=grpc:.