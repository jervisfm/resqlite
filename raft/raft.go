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

	"google.golang.org/grpc/codes"
	"sync"
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

	// Address information for this raft server node.
	localNode Node

	// Queue of event messages to be processed.
	events chan Event

	// Unix time in millis for when last hearbeat received when in non-leader
	// follower mode or when election started in candidate mode. Used to determine
	// when election timeouts occur.
	lastHeartbeatTimeMillis int64

	// True if we have received a heartbeat from a leader. Primary Purpose of this field to
	// determine whether we hear from a leader while in candidate status.
	receivedHeartbeat bool

	// Counts number of nodes in cluster that have chosen this node to be a leader
	receivedVoteCount int64
}

// Overall type for the messages processed by the event-loop.
type Event struct {
	// The RPC (Remote Procedure Call) to be handled.
	rpc RpcEvent
}

// Type holder RPC events to be processed.
type RpcEvent struct {
	requestVote* RaftRequestVoteRpcEvent
	appendEntries* RaftAppendEntriesRpcEvent
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
	electionTimeoutMillis int64

	// Amount of time in between heartbeat RPCs that leader sends. This should be
	// much less than electionTimeout to avoid risking starting new election due to slow
	// heartbeat RPCs.
	heartBeatIntervalMillis int64
}

// AppendEntries implementation for pb.RaftServer
func (s *Server) AppendEntries(ctx context.Context, in *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	replyChan := make(chan pb.AppendEntriesResponse)
	event := Event {
		rpc: RpcEvent{
			appendEntries: &RaftAppendEntriesRpcEvent{
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
			requestVote:&RaftRequestVoteRpcEvent{
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

// Starts a Raft Server listening at the specified local node.
// otherNodes contain contact information for other nodes in the cluster.
func StartServer(localNode Node, otherNodes []Node) *grpc.Server {
	addressPort := ":" + localNode.Port
	lis, err := net.Listen("tcp", addressPort)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	util.Log(util.DebugLevel, "Created Raft server at: %v", lis.Addr().String())
	s := grpc.NewServer()
	raftServer = GetInitialServer()
	log.Printf("Initial Server state: %v", raftServer)
	pb.RegisterRaftServer(s, &raftServer)
	// Register reflection service on gRPC server.
	reflection.Register(s)

	// Initialize raft cluster.
	raftServer.otherNodes = ConnectToOtherNodes(otherNodes)
	raftServer.localNode = localNode
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
			// TODO(jmuindi): Consider making this a command line flag.
			heartBeatIntervalMillis: 10,
		},
		events: make(chan Event),
		// We initialize last heartbeat time at startup because all servers start out
		// in follower and this allows a node to determine when it should be a candidate.
		lastHeartbeatTimeMillis: UnixMillis(),
	}
	return result
}

// Returns a go channel that blocks for a randomized election timeout time.
func RandomizedElectionTimeout() chan bool {
	waitDone := make(chan bool)
	timeoutMs := PickElectionTimeOutMillis()
	go func() {
		time.Sleep(time.Millisecond * time.Duration(timeoutMs))
		waitDone<- true
	}()
	return waitDone
}

// Picks a randomized time for the election timeout.
func PickElectionTimeOutMillis() int64 {
	baseTimeMs := int64(300)
	// Go random number is deterministic by default so we re-seed to get randomized behavior we want.
	rand.Seed(time.Now().Unix())
	randomOffsetMs := int64(rand.Intn(100))
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

// Returns the Hostname/IP:Port info for the local node. This serves as the
// identifier for the node.
func GetNodeId(node Node) string {
	return NodeToAddressString(node)
}

// Returns identifier for this server.
func GetLocalNodeId() string {
	return GetNodeId(raftServer.localNode)
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

// Resets the election time out. This restarts amount of time that has to pass
// before an election timeout occurs. Election timeouts lead to new elections.
func ResetElectionTimeOut() {
	raftServer.lastHeartbeatTimeMillis = UnixMillis()
}

// Returns true if this node already voted for a node to be a leader.
func AlreadyVoted() bool {
	if raftServer.raftState.persistentState.votedFor != "" {
		return true
	} else {
		return false
	}
}

func ChangeToCandidateStatus() {
	raftServer.serverState = Candidate
}

// Increments election term and also resets the relevant raft state.
func IncrementElectionTerm() {
	raftServer.raftState.persistentState.votedFor = ""
	raftServer.raftState.persistentState.currentTerm++
	raftServer.receivedVoteCount = 0
	raftServer.receivedHeartbeat = false
}

func VoteForSelf() {
	myId := GetLocalNodeId()
	raftServer.raftState.persistentState.votedFor = myId
	raftServer.receivedVoteCount++
}


// Votes for the given server node.
func VoteForServer(serverToVoteFor Node) {
	serverId := GetNodeId(serverToVoteFor)
	raftServer.raftState.persistentState.votedFor = serverId
}

// Returns the size of the raft cluster.
func GetRaftClusterSize() int64 {
	return int64(len(raftServer.otherNodes) + 1)
}


// Returns the number of votes needed to have a quorum in the cluster.
func GetQuorumSize() int64 {
	// Total := 2(N+1/2), where N is number of allowed failures.
	// Need N+1 for a quorum.
	// N: = (Total/2 - 0.5) = floor(Total/2)
	numTotalNodes := GetRaftClusterSize()
	quorumSize := (numTotalNodes / 2) + 1
	return quorumSize
}

func GetVoteCount() int64 {
	return raftServer.receivedVoteCount
}

// Returns true if this node has received sufficient votes to become a leader
func HaveEnoughVotes() bool {
	if GetVoteCount() >= GetQuorumSize() {
		return true
	} else {
		return false
	}
}

// Returns current raft term.
func RaftCurrentTerm() int64 {
	return raftServer.raftState.persistentState.currentTerm
}

func SetReceivedHeartBeat() {
	raftServer.receivedHeartbeat = true
}

// Instructions that followers would be processing.
func FollowerLoop() {

	// - Check if election timeout expired.
	// - If so, change to candidate status only.
	// Note(jmuindi):  The requirement to check that we have not already voted
	// as specified on figure 2 is covered because when after becoming a candidate
	// we vote for our self and the event loop code structure for rpcs processing
	// guarantees we won't vote for anyone else.

	for {
		if IsElectionTimeoutElapsed() {
			ChangeToCandidateStatus()
			return
		}

		// TODO(jmuindi): Process Any RPCs that we have.
		select {
		case event := <-raftServer.events:
			util.Log(util.VERBOSE, "Processing %v", event)
			handleRpcEvent(event)
		default:
			util.Log(util.VERBOSE, "No Events to process")
		}

	}


}

func handleRpcEvent(event Event) {
	if event.rpc.requestVote != nil {
		handleRequestVoteRpc(event.rpc.requestVote)
	} else if event.rpc.appendEntries != nil {
		handleAppendEntriesRpc(event.rpc.appendEntries)
	} else {
		log.Fatalf("Unexpected rpc event: %v", event)
	}
}

// Handles request vote rpc.
func handleRequestVoteRpc(event *RaftRequestVoteRpcEvent) {
	result := pb.RequestVoteResponse{}
	currentTerm := RaftCurrentTerm()
	result.Term = currentTerm
	if event.request.Term < currentTerm {
		result.VoteGranted = false
	} else if AlreadyVoted() {
		result.VoteGranted = false
	} else {
		// TODO(jmuindi): Only grant vote if candidate log at least uptodate
		// as receivers (Section 5.2; 5.4)
		result.VoteGranted = true
	}
	result.ResponseStatus = uint32(codes.OK)
	event.responseChan<- result
}

// Heartbeat sent by leader. Special case of Append Entries with no log entries.
func handleHeartBeatRpc(event *RaftAppendEntriesRpcEvent) {
	result := pb.AppendEntriesResponse{}
	currentTerm := RaftCurrentTerm()
	result.Term = currentTerm
	// Main thing is to reset the election timeout.
	result.ResponseStatus = uint32(codes.OK)
	ResetElectionTimeOut()
	SetReceivedHeartBeat()

	result.Success = true
	event.responseChan<- result
}

// Handles append entries rpc.
func handleAppendEntriesRpc(event *RaftAppendEntriesRpcEvent) {
	isHeartBeatRpc := len(event.request.Entries) == 0
	if isHeartBeatRpc {
		handleHeartBeatRpc(event)
		return
	}
	// Otherwise process regular append entries rpc
	// TODO(jmuindi): Implement

}

// Returns other nodes client connections
func GetOtherNodes() []pb.RaftClient {
	return raftServer.otherNodes
}


// Increments the number of received votes.
func IncrementVoteCount() {
	raftServer.receivedVoteCount++
}

// Returns the index of the last entry in the raft log. Index is 1-based.
func GetLastLogIndex() int64 {
	// TODO(jmuindi): Implement once we have raft log up.
	return 0
}

// Returns the term for the last entry in the raft log.
func GetLastLogTerm() int64 {
	// TODO(jmuindi): Implement once we have persistent log entry.
	return 0
}


// Requests votes from all the other nodes to make us a leader. Returns number of
// currently received votes
func RequestVotesFromOtherNodes() int64 {

	util.Log(util.INFO, "Have %v votes at start", GetVoteCount())
	otherNodes :=  GetOtherNodes()
	util.Log(util.INFO, "Requesting votes from other nodes: %v", GetOtherNodes())

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _, node := range otherNodes {
		go func() {
			defer waitGroup.Done()
			RequestVoteFromNode(node)
		}()
	}

	waitGroup.Wait()
	util.Log(util.INFO, "Have %v votes at end", GetVoteCount())
	return GetVoteCount()
}

// Requests a vote from the given node.
func RequestVoteFromNode(node pb.RaftClient) {
	voteRequest := pb.RequestVoteRequest{}
	voteRequest.Term = RaftCurrentTerm()
	voteRequest.CandidateId = GetLocalNodeId()
	voteRequest.LastLogIndex = GetLastLogIndex()
	voteRequest.LastLogTerm = GetLastLogTerm()

	result, err := node.RequestVote(context.Background(), &voteRequest)
	if err != nil {
		util.Log(util.ERROR, "Error getting vote from node %v err: %v", node, err)
		return
	}
	util.Log(util.VERBOSE, "Vote response: %v", *result)
	if result.VoteGranted {
		// TODO(jmuindi): Fix the race on incrementing the vote counter.
		IncrementVoteCount()
	}

}

// Changes to Follower status.
func ChangeToFollowerStatus() {
	raftServer.serverState = Follower
}

// Converts the node to a leader status from a candidate
func ChangeToLeaderStatus() {
	raftServer.serverState = Leader

}

// Instructions that candidate would be processing.
func CandidateLoop() {
	// High level notes overview:
	// Start an election process
	// - Increment current election term
	// - Vote for yourself
	// - Request votes in parallel from others nodes in cluster
	//
	// Remain a candidate until any of the following happens:
	// i) You win election (got enough votes) -> become leader
	// ii) Hear from another leader -> become follower
	// iii) A period of time goes by with no winner.
	for {
		if raftServer.serverState != Candidate {
			return
		}
		IncrementElectionTerm()
		VoteForSelf()
		RequestVotesFromOtherNodes()

		if HaveEnoughVotes() {
			ChangeToLeaderStatus()
			return
		}

		// If we don't have enough votes, it possible that:
		// a) Another node became a leader
		// b) Split votes, no node got majority.
		//
		// For both cases, wait out a little bit before starting another election.
		// This gives time to see if we hear from another leader (processing heartbeats)
		// and also reduces chance of continual split votes since each node has a random
		// timeout.
		timeoutDone := RandomizedElectionTimeout()
		for {
			if raftServer.receivedHeartbeat {
				// We have another leader and should convert to follower status.
				ChangeToFollowerStatus()
				return
			}
			select {
			case event := <-raftServer.events:
				handleRpcEvent(event)
			case <-timeoutDone:
				break

			}
		}
	}
}


// Reinitializes volatile leader state
func ReinitVolatileLeaderState() {

	// TODO(jmuindi): implement
}


func GetServerState() ServerState {
	return raftServer.serverState
}

// Returns the configured interval at which leader sends heartbeat rpcs.
func GetHeartbeatIntervalMillis() int64 {
	return raftServer.raftConfig.heartBeatIntervalMillis

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
	ReinitVolatileLeaderState()

	// Send heartbeats to followers in the background.
	go func() {
		for {
			if GetServerState() != Leader {
				util.Log(util.INFO, "No longer leader. Stopping heartbeat rpcs")
				return
			}
			SendHeartBeatsToFollowers()
			time.Sleep(time.Duration(GetHeartbeatIntervalMillis()) * time.Millisecond)
		}
	}()
}

// Send heart beat rpcs to followers in parallel and waits for them to all complete.
func SendHeartBeatsToFollowers() {
	util.Log(util.INFO, "Sending heartbeats to followers")
	otherNodes := GetOtherNodes()

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _,node := range otherNodes {
		// Send RPCs in parallel
		go func() {
			defer waitGroup.Done()
			SendHeartBeatRpc(node)
		}()
	}
	waitGroup.Wait()
}


// Sends a heartbeat rpc to the given raft node.
func SendHeartBeatRpc(node pb.RaftClient) {
	request := pb.AppendEntriesRequest{}
	request.Term = RaftCurrentTerm()
	request.LeaderId = GetLocalNodeId()
	request.LeaderCommit = GetLeaderCommit()
	request.PrevLogIndex = GetLeaderPreviousLogIndex()
	request.PrevLogTerm = GetLeaderPreviousLogTerm()
	// Log entries are empty/nil for heartbeat rpcs.
	request.Entries = nil

	result, err := node.AppendEntries(context.Background(), &request)
	if err != nil {
		util.Log(util.ERROR, "Error sending hearbeat to node: %v Error: %v", node, err)
		return
	}
	util.Log(util.EXTRA_VERBOSE, "Heartbeat RPC Response from node: %v Response: %v", node, *result)
}

// PrevLogTerm value  used in the appendentries rpc request.
func GetLeaderPreviousLogTerm() int64 {
	// TODO: implement
	return 0
}

// PrevLogIndex value  used in the appendentries rpc request.
func GetLeaderPreviousLogIndex() int64 {
	// TODO: implement
	return 0

}

// Leader commit value used in the appendentries rpc request.
func GetLeaderCommit() int64 {
	// TODO: implement
	return 0

}


// Overall loop for the server.
func StartServerLoop() {


	// TODO(jmuindi): The sub-loops below are busy looping which can burn CPU. Try finding
	// a way to make them more efficient. Ideas here include:
	// - Make _everything_ event based even the timeouts (may be more work than worth it).
	// - Add a polling frequency, e.g. wait 1ms
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
