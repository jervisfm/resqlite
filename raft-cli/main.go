//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"log"
	//"net"

	//"golang.org/x/net/context"
	//pb "github.com/jervisfm/resqlite/proto/raft"
	"github.com/jervisfm/resqlite/raft"
	//"google.golang.org/grpc"
	//"google.golang.org/grpc/reflection"
)


// Flags
var nodes string;
var port string;

func ParseFlags() {
	nodesPtr := flag.String("nodes", "", "A comma separated list of node IP:port addresses. The first node is presumed to be this node and the port number is what used to start the local raft server")

	flag.Parse()
	nodes = *nodesPtr
	//port = *portPtr
}


func main() {
	ParseFlags()
	log.Printf(" Starting Raft Server listening at: %v", port)
	log.Printf("Other Node ip address: %v", nodes)
	raft.StartServer(port)
}
