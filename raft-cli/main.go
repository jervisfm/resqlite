//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package main

import (
	//"log"
	//"net"

	//"golang.org/x/net/context"
	//pb "github.com/jervisfm/resqlite/proto/raft"
	"github.com/jervisfm/resqlite/raft"
	//"google.golang.org/grpc"
	//"google.golang.org/grpc/reflection"
)

const (
	port = ":50051"
)

func main() {
	raft.StartServer(port)
}
