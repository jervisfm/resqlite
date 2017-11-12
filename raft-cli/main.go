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
var nodes []raft.Node;
var port string;

func ParseFlags() {
	nodesPtr := flag.String("nodes", "", "A comma separated list of node IP:port addresses. The first node is presumed to be this node and the port number is what used to start the local raft server")
	flag.Parse()
	nodes = ParseNodes(*nodesPtr)
	port = GetLocalPort(nodes)
}


func GetLocalPort(nodes []raft.Node) (string){
	// The very first node is the local port value.
	return ":" + nodes[0].Port
}

// Returns other nodes in the cluster besides this one.
func GetOtherNodes() []raft.Node {
	result := append([]raft.Node(nil), nodes...)
	// Delete first element.
	result = append(result[:0], result[1:]...)
	return result
}

func ParseNodes(input string) ([]raft.Node) {
	pieces := strings.Split(input, ",")
	result := make([]raft.Node, 0)
	for _, nodeString := range pieces {
		result = append(result, ParseNodePortPairString(nodeString))
	}
	return result
}

func ParseNodePortPairString(input string) (raft.Node){
	pieces := strings.Split(input, ":")
	return raft.Node{Hostname: pieces[0], Port: pieces[1]}
}


func main() {
	ParseFlags()
	otherNodes := GetOtherNodes()
	log.Printf(" Starting Raft Server listening at: %v", port)
	log.Printf("All Node addresses: %v", nodes)
	log.Printf("Other Node addresses: %v", otherNodes)
	raft.StartServer(port, otherNodes)
}
