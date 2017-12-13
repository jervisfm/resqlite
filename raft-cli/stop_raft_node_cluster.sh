#!/usr/bin/env sh

# Stops any all running raft cluster nodes.

echo "Looking for raft cluster nodes ..."
ps | grep go | grep "main -nodes"

echo "Stopping all nodes ..."
ps | grep go | grep "main -nodes" | awk '{print $1}' | xargs kill -15
echo "Done"