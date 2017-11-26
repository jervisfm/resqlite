//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto

package raft

import (
	"log"
	"net"

	pb "github.com/jervisfm/resqlite/proto/raft"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"math/rand"
	"time"
	//"math/bits"
	"github.com/jervisfm/resqlite/util"
)

const (
	port = ":50051"
)

// Enum for the possible server states.
type ServerState int

const (
	// Followers only respond to request from other servers
	Follower = iota
	// Candidate is vying to become leaders
	Candidate
	// Leaders accept/process client process and continue until they fail.
	Leader
)

// server is used to implement pb.RaftServer
type Server struct {
	serverState ServerState

	raftConfig RaftConfig

	raftState RaftState

	// RPC clients for interacting with other nodes in the raft cluster.
	otherNodes []pb.RaftClient

	// Queue of event messages to be processed.
	events chan Event

	// Unix time in millis for when last hearbeat received when in non-leader
	// mode.
	lastHeartbeatTimeMillis int64
}

// Overall type for the messages processed by the event-loop.
type Event struct {
	// The RPC (Remote Procedure Call) to be handled.
	rpc RpcEvent
}

// Type holder RPC events to be processed.
type RpcEvent struct {
	requestVote RaftRequestVoteRpcEvent
	appendEntries RaftAppendEntriesRpcEvent
}

// Type for request vote rpc event.
type RaftRequestVoteRpcEvent struct {
	request pb.RequestVoteRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- pb.RequestVoteResponse
}

// Type for append entries rpc event.
type RaftAppendEntriesRpcEvent struct {
	request pb.AppendEntriesRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- pb.AppendEntriesResponse
}

// Contains all the inmemory state needed by the Raft algorithm
type RaftState struct {

	// TODO(jmuindi): Add support for real persistent state; perhaps we can use a sqlite db underneath?
	persistentState     RaftPersistentState
	volatileState       RaftVolatileState
	volatileLeaderState RaftLeaderState
}

type RaftPersistentState struct {
	currentTerm int64
	votedFor    string
	log         []string
}

type RaftVolatileState struct {
	commitIndex int64
	lastApplied int64
}

type RaftLeaderState struct {
	nextIndex  []int64
	matchIndex []int64
}

// Contains Raft configuration parameters
type RaftConfig struct {

	// Amount of time to wait before starting an election.
	electionTimeoutMillis int
}

// AppendEntries implementation for pb.RaftServer
func (s *Server) AppendEntries(ctx context.Context, in *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	replyChan := make(chan pb.AppendEntriesResponse)
	event := Event {
		rpc: RpcEvent{
			appendEntries: RaftAppendEntriesRpcEvent{
				request: *in,
				responseChan: replyChan,
			},
		},
	}
	raftServer.events<- event

	result := <-replyChan
	return &result, nil
}

// RequestVote implementation for raft.RaftServer
func (s *Server) RequestVote(ctx context.Context, in *pb.RequestVoteRequest) (*pb.RequestVoteResponse, error) {
	replyChan := make(chan pb.RequestVoteResponse)
	event := Event{
		rpc: RpcEvent{
			requestVote:RaftRequestVoteRpcEvent{
				request: *in,
				responseChan: replyChan,
			},
		},
	}
	raftServer.events<- event

	result := <-replyChan
	return &result, nil
}

// Specification for a node
type Node struct {
	// A hostanme of the node either in DNS or IP form e.g. localhost
	Hostname string
	// A port number for the node. e.g. :50051
	Port string
}

// Variables

// Handle to the raft server.
var raftServer Server

// Connects to a Raft server listening at the given address and returns a client
// to talk to this server.
func ConnectToServer(address string) pb.RaftClient {
	// Set up a connection to the server. Note: this is not a blocking call.
	// Connection will be setup in the background.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	c := pb.NewRaftClient(conn)

	return c
}

// Starts a Raft Server listening at the specified address port. (e.g. :50051).
// otherNodes contain contact information for other nodes in the cluster.
func StartServer(addressPort string, otherNodes []Node) *grpc.Server {
	lis, err := net.Listen("tcp", addressPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	raftServer = GetInitialServer()
	log.Printf("Initial Server state: %v", raftServer)
	pb.RegisterRaftServer(s, &raftServer)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	// Intialize raft cluster.
	raftServer.otherNodes = ConnectToOtherNodes(otherNodes)
	go InitializeRaft(addressPort, otherNodes)

	// Note: the Serve call is blocking.
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

	return s
}

// Returns initial server state.
func GetInitialServer() Server {
	result := Server{
		serverState: Follower,
		raftConfig: RaftConfig{
			electionTimeoutMillis: PickElectionTimeOutMillis(),
		},
		events: make(chan Event),
		// We initialize last heartbeat time at startup because all servers start out
		// in follower and this allows a node to determine when it should be a candidate.
		lastHeartbeatTimeMillis: UnixMillis(),
	}
	return result
}

// Picks a randomized time for the election timeout.
func PickElectionTimeOutMillis() int {
	baseTimeMs := 300
	// Go random number is deterministic by default so we re-seed to get randomized behavior we want.
	rand.Seed(time.Now().Unix())
	randomOffsetMs := rand.Intn(100)
	return baseTimeMs + randomOffsetMs
}

func NodeToAddressString(input Node) string {
	return input.Hostname + ":" + input.Port
}

// Connects to the other Raft nodes and returns array of Raft Client connections.
func ConnectToOtherNodes(otherNodes []Node) []pb.RaftClient {

	result := make([]pb.RaftClient, 0)
	for _, node := range otherNodes {
		serverAddress := NodeToAddressString(node)
		log.Printf("Connecting to server: %v", serverAddress)
		client := ConnectToServer(serverAddress)
		result = append(result, client)
	}
	return result
}

func TestNodeConnections(nodeConns []pb.RaftClient) {
	// Try a test RPC call to other nodes.
	log.Printf("Have client conns: %v", nodeConns)
	for _, nodeConn := range nodeConns {
		result, err := nodeConn.RequestVote(context.Background(), &pb.RequestVoteRequest{})
		if err != nil {
			log.Printf("Error on connection: %v", err)
		}
		log.Printf("Got Response: %v", result)
	}

}

// Initializes Raft on server startup.
func InitializeRaft(addressPort string, otherNodes []Node) {
	StartServerLoop()
}

// Returns duration of time in milliseconds since the last successful heartbeat.
func TimeSinceLastHeartBeatMillis() int64 {
	now := UnixMillis()
	diffMs :=  now - raftServer.lastHeartbeatTimeMillis
	if (diffMs < 0) {
		util.Log(util.WARN, "Negative time since last heartbeat. Assuming 0.")
		diffMs = 0
	}
	return diffMs
}

// Returns true if the election timeout has already passed for this node.
func IsElectionTimeoutElapsed() bool {
	timeoutMs := raftServer.raftConfig.electionTimeoutMillis
	elapsedMs := TimeSinceLastHeartBeatMillis()
	if (elapsedMs > timeoutMs) {
		return true
	} else {
		return false
	}
}

// Instructions that followers would be processing.
func FollowerLoop() {

	// TOOD(jmuindi): implement.

	// - Check if election timeout expired.
	// - If so, change to candidate status only iff you have not yet voted for
	// another node.

}

// Instructions that candidate would be processing.
func CandidateLoop() {
	// TOOD(jmuindi): implement.

	// High level notes overview:
	// Start an election process
	// - Increment current election term.
	// - Vote for yourself
	// - Request votes in parallel from others nodes in cluster
	//
	// Remain a candidate until any of the following happens:
	// i) You win election (got enough votes)
	// ii) Hear from another leader
	// iii) A period of time goes by with no winner.
}

// Instructions that leaders would be performing.
func LeaderLoop() {
	// TOOD(jmuindi): implement.
	// Overview:
	// - Reinitialize volatile leader state upon first leader succession.
	// - Send initial empty append entries rpcs to clients as heartbeats. Repeat
	//   to avoid election timeout.
	// - Process commands from end-user clients. Respond after data replicated on
	//   majority of nodes. i.e. append to local log, respond after entry applied to
	//   state machine.
	// - See Figure 2 from Raft paper for 2 other leader requirements.
}

// Overall loop for the server.
func StartServerLoop() {

	for {
		if (raftServer.serverState == Leader) {
			LeaderLoop()
		} else if (raftServer.serverState == Follower) {
			FollowerLoop()
		} else if (raftServer.serverState == Candidate) {
			CandidateLoop()
		} else {
			log.Fatalf("Unexpected / unknown server state: %v", raftServer.serverState)
		}
	}
}

// Returns the current time since unix epoch in milliseconds.
func UnixMillis() int64 {
	now := time.Now()
	unixNano := now.UnixNano()
	unixMillis := unixNano / 1000000
	return unixMillis
}
