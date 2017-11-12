//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	"flag"
	"log"
	//"net"
"strings"
	//"golang.org/x/net/context"
	//pb "github.com/jervisfm/resqlite/proto/raft"
	"github.com/jervisfm/resqlite/raft"
	//"google.golang.org/grpc"
	//"google.golang.org/grpc/reflection"
)


// Flags
var nodes []Node;
var port string;

func ParseFlags() {
	nodesPtr := flag.String("nodes", "", "A comma separated list of node IP:port addresses. The first node is presumed to be this node and the port number is what used to start the local raft server")
	flag.Parse()
	nodes = ParseNodes(*nodesPtr)
	port = GetLocalPort(nodes)
}

type Node struct {
	// A hostanme of the node either in DNS or IP form e.g. localhost
	hostname string
	// A port number for the node. e.g. :50051
	port string
}

func GetLocalPort(nodes []Node) (string){
	// The very first node is the local port value.
	return nodes[0].port
}

func ParseNodes(input string) ([]Node) {
	pieces := strings.Split(input, ",")
	result := make([]Node, 0)
	for _, nodeString := range pieces {
		result = append(result, ParseNodePortPairString(nodeString))
	}
	return result
}

func ParseNodePortPairString(input string) (Node){
	pieces := strings.Split(input, ":")
	return Node{hostname: pieces[0], port: pieces[1]}
}


func main() {
	ParseFlags()
	log.Printf(" Starting Raft Server listening at: %v", port)
	log.Printf("Other Node ip address: %v", nodes)
	raft.StartServer(port)
}
