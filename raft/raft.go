//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	//"log"
	//"net"

	pb "github.com/jervisfm/resqlite/proto/raft"
	"golang.org/x/net/context"
	//"google.golang.org/grpc"
	//"google.golang.org/grpc/reflection"
)

const (
	port = ":50051"
)

// server is used to implement raft.RaftServer
type Server struct{}

// AppendEntries implements raft.RaftServer
func (s *Server) AppendEntries(ctx context.Context, in *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	// TODO(jmuindi): Implement.
	return &pb.AppendEntriesResponse{}, nil
}

func (s *Server) RequestVote(ctx context.Context, in *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	// TODO(jmuindi): Implement.
	return &pb.RequestVoteResponse{}, nil
}
