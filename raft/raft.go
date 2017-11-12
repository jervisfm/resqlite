//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	"log"
	"net"



	pb "github.com/jervisfm/resqlite/proto/raft"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

const (
	port = ":50051"
)

// server is used to implement pb.RaftServer
type Server struct{}

// AppendEntries implementation for pb.RaftServer
func (s *Server) AppendEntries(ctx context.Context, in *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	// TODO(jmuindi): Implement.
	return &pb.AppendEntriesResponse{}, nil
}

// RequestVote implementation for raft.RaftServer
func (s *Server) RequestVote(ctx context.Context, in *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	// TODO(jmuindi): Implement.
	return &pb.RequestVoteResponse{}, nil
}

// Specification for a node
type Node struct {
	// A hostanme of the node either in DNS or IP form e.g. localhost
	Hostname string
	// A port number for the node. e.g. :50051
	Port string
}


// Variables

// Tracks connections to other raft nodes.
var nodeConns []pb.RaftClient

// Connects to a Raft server listening at the given address and returns a client
// to talk to this server.
func ConnectToServer(address string) (pb.RaftClient) {
        // Set up a connection to the server.
        conn, err := grpc.Dial(address, grpc.WithInsecure())
        if err != nil {
                log.Fatalf("did not connect: %v", err)
        }
        c := pb.NewRaftClient(conn)

	return c
}

// Starts a Raft Server listening at the specified address port. (e.g. :50051).
// otherNodes contain contact information for other nodes in the cluster.
func StartServer(addressPort string, otherNodes []Node) (*grpc.Server) {
	lis, err := net.Listen("tcp", addressPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterRaftServer(s, &Server{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	// 
	return s
}


