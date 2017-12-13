#!/usr/bin/env sh

# Start up 3 node raft cluster.

./node3.sh >/tmp/node3_output.txt &
echo "Started Node 3"

./node2.sh >/tmp/node2_output.txt &
echo "Started Node 2"

./node1.sh >/tmp/node1_output.txt &
echo "Started Node 1"
