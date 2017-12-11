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
	"sync/atomic"

	"database/sql"

	// Import for sqlite3 support.
	_ "github.com/mattn/go-sqlite3"
	"strings"
	"github.com/golang/protobuf/proto"
	"strconv"
	"math"
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

	// Database containing the persistent raft log
	raftLogDb *sql.DB

	// sqlite Database of the replicated state machine.
	sqlDb * sql.DB

	// Mutex to synchronize concurrent access to data structure
	lock sync.Mutex
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
	clientCommand* RaftClientCommandRpcEvent
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

// Type for client command rpc event.
type RaftClientCommandRpcEvent struct {
	request pb.ClientCommandRequest
	// Channel for event loop to communicate back response to client.
	responseChan chan<- pb.ClientCommandResponse
}

// Contains all the inmemory state needed by the Raft algorithm
type RaftState struct {

	persistentState     RaftPersistentState
	volatileState       RaftVolatileState
	volatileLeaderState RaftLeaderState
}

type RaftPersistentState struct {
	currentTerm int64
	votedFor    string
	log         []pb.DiskLogEntry
}

type RaftVolatileState struct {
	commitIndex int64
	lastApplied int64

	// Custom fields

	// Id for who we believe to be the current leader of the cluster.
	leaderId string
}

type RaftLeaderState struct {

	// Index of the next log entry to send to that server. Should be initialized
	// at leader last log index + 1.
	nextIndex  []int64

	// Index of the highest log entry known to be replicated on each server.
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

// Raft Persistent State accessors / getters / functions.
func GetPersistentRaftLog() []pb.DiskLogEntry {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return raftServer.raftState.persistentState.log
}

func GetPersistentVotedFor() string {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	return GetPersistentVotedForLocked()
}

// Locked means caller already holds appropriate lock.
func GetPersistentVotedForLocked() string {
	return raftServer.raftState.persistentState.votedFor
}


func SetPersistentVotedFor(newValue string)  {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	SetPersistentVotedForLocked(newValue)
}

func SetPersistentVotedForLocked(newValue string)  {
	// Want to write to stable storage (database).
	// First determine if we're updating or inserting value.

	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'votedFor'")
	if err != nil {
		log.Fatalf("Failed to read persisted voted for while trying to update it. err: %v", err)
	}
	defer rows.Close()
	votedForExists := false
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Fatalf("Error  reading persisted voted for row while try to update: %v", err)
		}
		votedForExists = true
	}

	// Now proceed with the update/insertion.
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Fatalf("Failed to begin db tx to update voted for. err:%v", err)
	}

	needUpdate := votedForExists
	var statement *sql.Stmt
	if needUpdate {
		statement, err = tx.Prepare("UPDATE RaftKeyValue SET value = ? WHERE key = ?")
		if err != nil {
			log.Fatalf("Failed to prepare stmt to update voted for. err: %v", err)
		}
		_, err = statement.Exec(newValue, "votedFor")
		if err != nil {
			log.Fatalf("Failed to update voted for value. err: %v", err)
		}
	} else {
		statement, err = tx.Prepare("INSERT INTO RaftKeyValue(key, value) values(?, ?)")
		if err != nil {
			log.Fatalf("Failed to create stmt to insert voted for. err: %v", err)
		}
		_, err = statement.Exec("votedFor", newValue)
		if err != nil {
			log.Fatalf("Failed to insert voted for value. err: %v", err)
		}
	}
	defer statement.Close()
	err = tx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit tx to update voted for. err: %v", err)
	}

	// Then update in-memory state last.
	raftServer.raftState.persistentState.votedFor = newValue
}

func GetPersistentCurrentTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return GetPersistentCurrentTermLocked()
}

func GetPersistentCurrentTermLocked() int64 {
	return raftServer.raftState.persistentState.currentTerm
}

func SetPersistentCurrentTerm(newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	SetPersistentCurrentTermLocked(newValue)
}

func SetPersistentCurrentTermLocked(newValue int64) {
	// Write to durable storage (database) first.
	// First determine if we're updating or inserting brand new value.

	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'currentTerm'")
	if err != nil {
		log.Fatalf("Failed to read persisted current term while trying to update it. err: %v", err)
	}
	defer rows.Close()
	valueExists := false
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Fatalf("Error reading persisted currentTerm row while try to update: %v", err)
		}
		valueExists = true
	}

	// Now proceed with the update/insertion.

	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Fatalf("Failed to begin db tx to update current term. err:%v", err)
	}

	newValueStr := strconv.FormatInt(newValue, 10)
	var statement *sql.Stmt
	if valueExists {
		// Then update it.
		statement, err = tx.Prepare("UPDATE RaftKeyValue SET value = ? WHERE key = ?")
		if err != nil {
			log.Fatalf("Failed to prepare stmt to update current term. err: %v", err)
		}
		_, err = statement.Exec(newValueStr, "currentTerm")
		if err != nil {
			log.Fatalf("Failed to update current term value. err: %v", err)
		}
	} else {
		// Insert a brand new one.
		statement, err = tx.Prepare("INSERT INTO RaftKeyValue(key, value) values(?, ?)")
		if err != nil {
			log.Fatalf("Failed to create stmt to insert current term. err: %v", err)
		}
		_, err = statement.Exec("currentTerm", newValueStr)
		if err != nil {
			log.Fatalf("Failed to insert currentTerm value. err: %v", err)
		}
	}
	defer statement.Close()
	err = tx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit tx to update current term. err: %v", err)
	}

	// Then update in memory state last.
	raftServer.raftState.persistentState.currentTerm = newValue
}

func AddPersistentLogEntry(newValue pb.LogEntry) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	AddPersistentLogEntryLocked(newValue)
}

func AddPersistentLogEntryLocked(newValue pb.LogEntry) {
	nextIndex := int64(len(raftServer.raftState.persistentState.log)) + 1
	diskEntry := pb.DiskLogEntry{
		LogEntry: &newValue,
		LogIndex: nextIndex,
	}

	// Update database (stable storage first).
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Fatalf("Failed to begin db tx. Err: %v", err)
	}
	statement, err := tx.Prepare("INSERT INTO RaftLog(log_index, log_entry) values(?, ?)")
	if err != nil {
		log.Fatalf("Failed to prepare sql statement to add log entry. err: %v", err)
	}
	defer statement.Close()
	protoText := proto.MarshalTextString(&newValue)
	_, err = statement.Exec(nextIndex, protoText)
	if err != nil {
		log.Fatalf("Failed to execute sql statement to add log entry. err: %v", err)
	}
	err = tx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit tx to add log entry. err: %v", err)
	}

	raftServer.raftState.persistentState.log = append(raftServer.raftState.persistentState.log, diskEntry)
}

func DeletePersistentLogEntryInclusive(startDeleteLogIndex int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	DeletePersistentLogEntryInclusiveLocked(startDeleteLogIndex)
}

// Delete all log entries starting from the given log index.
// Note: input log index is 1-based.
func DeletePersistentLogEntryInclusiveLocked(startDeleteLogIndex int64) {

	// Delete first from database storage.
	tx, err := raftServer.raftLogDb.Begin()
	if err != nil {
		log.Fatalf("Failed to begin db tx for delete log entries. err: %v", err)
	}

	statement, err := tx.Prepare("DELETE FROM RaftLog WHERE log_index >= ?")
	if err != nil {
		log.Fatalf("Failed to prepare sql statement to delete log entry. err: %v", err)
	}
	defer statement.Close()

	_, err = statement.Exec(startDeleteLogIndex)
	if err != nil {
		log.Fatalf("Failed to execute sql statement to delete log entry. err: %v", err)
	}
	err = tx.Commit()
	if err != nil {
		log.Fatalf("Failed to commit tx to delete log entry. err: %v", err)
	}

	// Finally update the in-memory state.
	zeroBasedDeleteIndex := startDeleteLogIndex - 1
	raftServer.raftState.persistentState.log = append(raftServer.raftState.persistentState.log[:zeroBasedDeleteIndex])
}


func IncrementPersistenCurrentTerm() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	IncrementPersistentCurrentTermLocked()
}

func IncrementPersistentCurrentTermLocked() {
	val := GetPersistentCurrentTermLocked()
	newVal := val + 1
	SetPersistentCurrentTermLocked(newVal)
}


func SetReceivedHeartbeat(newVal bool) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	SetReceivedHeartbeatLocked(newVal)
}

func SetReceivedHeartbeatLocked(newVal bool) {
	raftServer.receivedHeartbeat = newVal
}


// Raft Volatile State.
func GetLeaderIdLocked() string {
	return raftServer.raftState.volatileState.leaderId
}

func GetLeaderId() string {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return GetLeaderIdLocked()
}

func SetLeaderIdLocked(newVal string) {
	raftServer.raftState.volatileState.leaderId = newVal
}

func SetLeaderId(newVal string) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()
	SetLeaderIdLocked(newVal)
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

// Client Command implementation for raft.RaftServer
func (s *Server) ClientCommand(ctx context.Context, in *pb.ClientCommandRequest) (*pb.ClientCommandResponse, error) {
	replyChan := make(chan pb.ClientCommandResponse)
	event := Event{
		rpc: RpcEvent{
			clientCommand: &RaftClientCommandRpcEvent{
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
	// A hostname of the node either in DNS or IP form e.g. localhost
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

// Returns true iff server is the leader for the cluster.
func IsLeader() bool {
	return GetServerState() == Leader
}

// Returns true iff server is a follower in the cluster
func IsFollower() bool {
	return GetServerState() == Follower
}

// Returns true iff server is a candidate
func IsCandidate() bool {
	return GetServerState() == Candidate
}

// Returns initial server state.
func GetInitialServer() Server {
	result := Server{
		serverState: Follower,
		raftConfig: RaftConfig{
			electionTimeoutMillis: PickElectionTimeOutMillis(),
			heartBeatIntervalMillis: 10,
		},
		events: make(chan Event),
		// We initialize last heartbeat time at startup because all servers start out
		// in follower and this allows a node to determine when it should be a candidate.
		lastHeartbeatTimeMillis: UnixMillis(),
	}
	return result
}

// Returns a go channel that blocks for specified amount of time.
func GetTimeoutWaitChannel(timeoutMs int64) *time.Timer {
	return time.NewTimer(time.Millisecond * time.Duration(timeoutMs))
}


func GetConfigElectionTimeoutMillis() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftConfig.electionTimeoutMillis
}

// Get amount of time remaining in millis before last heartbeat received is considered
// to have expired and thus we no longer have a leader.
func GetRemainingHeartbeatTimeMs() int64 {
	timeoutMs := GetConfigElectionTimeoutMillis()
	elapsedMs := TimeSinceLastHeartBeatMillis()
	remainingMs := timeoutMs - elapsedMs
	if (remainingMs < 0) {
		remainingMs = 0
	}
	return remainingMs
}


// Returns a go channel that blocks for a randomized election timeout time.
func RandomizedElectionTimeout() *time.Timer {
	timeoutMs := PickElectionTimeOutMillis()
	return GetTimeoutWaitChannel(timeoutMs)
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

func GetLocalNode() Node {
	// No lock on localNode as it's unchanged after server init.

	return raftServer.localNode
}

// Returns identifier for this server.
func GetLocalNodeId() string {
	return GetNodeId(GetLocalNode())
}

// Initializes Raft on server startup.
func InitializeRaft(addressPort string, otherNodes []Node) {
	InitializeDatabases()
	StartServerLoop()
}

func InitializeDatabases() {
	raftDbLog, err := sql.Open("sqlite3" , GetSqliteRaftLogPath())
	if err != nil {
		log.Fatalf("Failed to open raft db log: %v err: %v", GetSqliteRaftLogPath(), err)
	}
	replicatedStateMachineDb, err := sql.Open("sqlite3", GetSqliteReplicatedStateMachineOpenPath())
	if err != nil {
		log.Fatalf("Failed to open replicated state machine database. Path: %v err: %v", GetSqliteReplicatedStateMachineOpenPath(), err)
	}

	raftDbLog.Exec("pragma database_list")
	replicatedStateMachineDb.Exec("pragma database_list")

	// Create Tables for the raft log persistence. We want two tables:
	// 1) RaftLog with columns of log_index, log entry proto
	// 2) RaftKeyValue with columns of key,value. This is used to keep two properties
	//    votedFor and currentTerm.

	raftLogTableCreateStatement :=
		` CREATE TABLE IF NOT EXISTS RaftLog (
             log_index INTEGER NOT NULL PRIMARY KEY,
             log_entry TEXT);
        `
	_, err = raftDbLog.Exec(raftLogTableCreateStatement)
	if err != nil {
		log.Fatalf("Failed to Create raft log table. sql: %v err: %v", raftLogTableCreateStatement, err)
	}

	raftKeyValueCreateStatement :=
		` CREATE TABLE IF NOT EXISTS RaftKeyValue (
              key TEXT NOT NULL PRIMARY KEY,
              value TEXT);
        `
	_, err = raftDbLog.Exec(raftKeyValueCreateStatement)
     if err != nil {
     	log.Fatalf("Failed to create raft key value table. sql: %v, err: %v", raftKeyValueCreateStatement, err)
	 }

	 raftServer.sqlDb = replicatedStateMachineDb
	 raftServer.raftLogDb = raftDbLog

	 LoadPersistentStateIntoMemory()
}

// Loads the on disk persistent state into memory.
func LoadPersistentStateIntoMemory() {
	util.Log(util.INFO, "Before load. Raft Persistent State: %v ", raftServer.raftState.persistentState)
	LoadPersistentLog()
	LoadPersistentKeyValues()
	util.Log(util.INFO, "After load. Raft Persistent State: %v ", raftServer.raftState.persistentState)
}

func LoadPersistentKeyValues() {

	// Load value for the current term.
	LoadPersistedCurrentTerm()

	// Load value for the voted for into memory
	LoadPersistedVotedFor()

}
func LoadPersistedVotedFor() {
	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'votedFor';")
	if err != nil {
		log.Fatalf("Failed to read votedFor value into memory. err: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Fatalf("Failed to read votedFor row entry. err: %v", err)
		}

		util.Log(util.INFO, "Restoring votedFor value to memory: %v", valueStr)
		raftServer.raftState.persistentState.votedFor = valueStr
	}
}

func LoadPersistedCurrentTerm() {
	rows, err := raftServer.raftLogDb.Query("SELECT value FROM RaftKeyValue WHERE key = 'currentTerm';")
	if err != nil {
		log.Fatalf("Failed to read current term value into memory. err: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var valueStr string
		err = rows.Scan(&valueStr)
		if err != nil {
			log.Fatalf("Failed to read current term row entry. err: %v", err)
		}
		restoredTerm, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			log.Fatalf("Failed to parse current term as integer. value: %v err: %v", valueStr, err)
		}
		util.Log(util.INFO, "Restoring current term to memory: %v", restoredTerm)
		raftServer.raftState.persistentState.currentTerm = restoredTerm
	}
}

func LoadPersistentLog() {
	rows, err := raftServer.raftLogDb.Query("SELECT log_index, log_entry FROM RaftLog ORDER BY log_index ASC;")
	if err != nil {
		log.Fatalf("Failed to load raft persistent state into memory. err: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var logIndex int64
		var logEntryProtoText string
		err = rows.Scan(&logIndex, &logEntryProtoText)
		if err != nil {
			log.Fatalf("Failed to read raft log row entry. err: %v ", err)
		}
		util.Log(util.INFO, "Restoring raft log to memory: %v, %v", logIndex, logEntryProtoText)

		var parsedLogEntry pb.LogEntry
		err := proto.UnmarshalText(logEntryProtoText, &parsedLogEntry)
		if err != nil {
			log.Fatalf("Error parsing log entry proto: %v err: %v", logEntryProtoText, err)
		}
		diskLogEntry := pb.DiskLogEntry{
			LogIndex: logIndex,
			LogEntry: &parsedLogEntry,
		}

		raftServer.raftState.persistentState.log = append(raftServer.raftState.persistentState.log, diskLogEntry)
	}
}

// Moves the commit index forward from the current value to the given index.
// Note: newIndex should be the log index value which is 1-based.
func MoveCommitIndexTo(newIndex int64) {
	util.Log(util.WARN, "(UNIMPLEMENTED) MoveCommitIndexTo newIndex: %v", newIndex)

	startCommitIndex := GetCommitIndex()
	newCommitIndex := newIndex

	if newCommitIndex < startCommitIndex {
		log.Fatalf("Commit index trying to move backwards. ")
	}

	startCommitIndexZeroBased := startCommitIndex - 1
	newCommitIndexZeroBased := newCommitIndex - 1

	raftLog := GetPersistentRaftLog()
	commands := raftLog[startCommitIndexZeroBased + 1 : newCommitIndexZeroBased + 1]
	for _, cmd := range commands {
		if newCommitIndex > GetLastApplied() {
			util.Log(util.INFO, "Applying log entry: %v", cmd)
			ApplySqlCommand(cmd.LogEntry.Data)
			SetLastApplied(cmd.LogIndex)
		}
	}
	SetLastApplied(newCommitIndex)

	// Finally update the commit index.
	SetCommitIndex(newCommitIndex)
}

func GetLastHeartbeatTimeMillis() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.lastHeartbeatTimeMillis
}

// Returns duration of time in milliseconds since the last successful heartbeat.
func TimeSinceLastHeartBeatMillis() int64 {
	now := UnixMillis()
	diffMs :=  now - GetLastHeartbeatTimeMillis()
	if (diffMs < 0) {
		util.Log(util.WARN, "Negative time since last heartbeat. Assuming 0.")
		diffMs = 0
	}
	return diffMs
}

// Returns true if the election timeout has already passed for this node.
func IsElectionTimeoutElapsed() bool {
	timeoutMs := GetConfigElectionTimeoutMillis()
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
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.lastHeartbeatTimeMillis = UnixMillis()
}

// Returns true if this node already voted for a node to be a leader.
func AlreadyVoted() bool {

	if GetPersistentVotedFor() != "" {
		return true
	} else {
		return false
	}
}

func ChangeToCandidateStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.serverState = Candidate
}

func GetReceivedHeartbeat() bool {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.receivedHeartbeat
}

func ResetReceivedVoteCount() {
	// Using atomics -- so no need to lock.
	atomic.StoreInt64(&raftServer.receivedVoteCount, 0)
}

// Increments election term and also resets the relevant raft state.
func IncrementElectionTerm() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()


	SetPersistentVotedForLocked("")
	IncrementPersistentCurrentTermLocked()
	ResetReceivedVoteCount()
	SetReceivedHeartbeatLocked(false)
}

func VoteForSelf() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	myId := GetLocalNodeId()
	SetPersistentVotedForLocked(myId)
	IncrementVoteCount()
}


// Votes for the given server node.
func VoteForServer(serverToVoteFor Node) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	serverId := GetNodeId(serverToVoteFor)
	SetPersistentVotedForLocked(serverId)
}

// Returns the size of the raft cluster.
func GetRaftClusterSize() int64 {
	// No lock here because otherNote is unchanged after server init.

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
	return atomic.LoadInt64(&raftServer.receivedVoteCount)
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
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return GetPersistentCurrentTermLocked()
}

func SetReceivedHeartBeat() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

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
	util.Log(util.INFO, "Starting  follower loop")
	ResetElectionTimeOut()
	rpcCount := 0
	for {
		if GetServerState() != Follower {
			return
		}

		remainingHeartbeatTimeMs := GetRemainingHeartbeatTimeMs()
		timeoutTimer := GetTimeoutWaitChannel(remainingHeartbeatTimeMs)

		select {
		case event := <-raftServer.events:
			util.Log(util.VERBOSE, "Processing rpc #%v event: %v", rpcCount, event)
			handleRpcEvent(event)
			rpcCount++
		case <-timeoutTimer.C:
			// Election timeout occured w/o heartbeat from leader.
			ChangeToCandidateStatus()
			return
		}
	}
}

func handleRpcEvent(event Event) {
	if event.rpc.requestVote != nil {
		handleRequestVoteRpc(event.rpc.requestVote)
	} else if event.rpc.appendEntries != nil {
		handleAppendEntriesRpc(event.rpc.appendEntries)
	} else if event.rpc.clientCommand != nil {
		handleClientCommandRpc(event.rpc.clientCommand)
	} else {
		log.Fatalf("Unexpected rpc event: %v", event)
	}
}

// Handles client command request.
func handleClientCommandRpc(event *RaftClientCommandRpcEvent) {
	if !IsLeader() {
		result := pb.ClientCommandResponse{}
		util.Log(util.WARN, "Rejecting client command because not leader")
		result.ResponseStatus = uint32(codes.FailedPrecondition)
		result.NewLeaderId = GetLeaderId()
		event.responseChan<- result
		return
	}

	if event.request.GetCommand() != "" {
		handleClientMutateCommand(event)
	} else if event.request.GetQuery() != "" {
		handleClientQueryCommand(event)
	} else {
		// Invalid / unexpected request.
		result := pb.ClientCommandResponse{}
		util.Log(util.WARN, "Invalid client command (not command/query): %v", event)
		result.ResponseStatus = uint32(codes.InvalidArgument)
		event.responseChan<- result
		return
	}

}

func handleClientQueryCommand(event *RaftClientCommandRpcEvent) {
	sqlQuery := event.request.Query
	util.Log(util.INFO, "Servicing SQL query: %v", sqlQuery)

	result := pb.ClientCommandResponse{}

	rows, err := raftServer.sqlDb.Query(sqlQuery)
	if err != nil {
		util.Log(util.WARN, "Sql query error: %v", err)
		result.ResponseStatus = uint32(codes.Aborted)
		event.responseChan <- result
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		util.Log(util.WARN, "Sql cols query error: %v", err)
		result.ResponseStatus = uint32(codes.Aborted)
		event.responseChan <- result
		return
	}

	columnData := make([]string, len(columns))

	rawData := make([][]byte , len(columns))
	tempData := make([]interface{}, len(columns))
	for i, _ := range rawData {
		tempData[i] = &rawData[i]
	}

	for rows.Next() {
		err = rows.Scan(tempData...)
		if err != nil {
			util.Log(util.WARN, "Sql query error. Partial data return: %v", err)
			continue
		}

		for i, val := range rawData {
			if val == nil {
				columnData[i] = ""
			} else {
				columnData[i] = string(val)
			}
		}
	}

	// Combine column data into one string.
	queryResult := strings.Join(columnData, " | ")
	result.QueryResponse = queryResult

	result.ResponseStatus = uint32(codes.OK)
	event.responseChan <- result

}

func handleClientMutateCommand(event *RaftClientCommandRpcEvent) {
	result := pb.ClientCommandResponse{}
	// From Section 5.3 We need to do the following
	// 1) Append command to our log as a new entry
	// 2) Issue AppendEntries RPC in parallel to each of the other other nodes
	// 3) When Get majority successful responses, apply the new entry to our state
	//   machine, and reply to client.
	// Note: If some other followers slow, we apply append entries rpcs indefinitely.
	appendCommandToLocalLog(event)
	replicationSuccess := IssueAppendEntriesRpcToMajorityNodes(event)
	if replicationSuccess {
		result.ResponseStatus = uint32(codes.OK)
		ApplyCommandToStateMachine(event)
	} else {
		result.ResponseStatus = uint32(codes.Aborted)
	}
	event.responseChan <- result
}


// Applies the command to the local state machine. For us this, this is to apply the
// sql command.
func ApplyCommandToStateMachine(event *RaftClientCommandRpcEvent) {
	util.Log(util.INFO, "Update State machine with command: %v", event.request.Command)
	ApplySqlCommand(event.request.Command)
}

func ApplySqlCommand(sqlCommand string) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	ApplySqlCommandLocked(sqlCommand)
}

func ApplySqlCommandLocked(sqlCommand string) {
	_, err := raftServer.sqlDb.Exec(sqlCommand)
	if err != nil {
		util.Log(util.WARN, "Sql application execution warning: %v", err)
	}
}


// sql database which is the state machine for this node.
func GetSqliteReplicatedStateMachineOpenPath() string {
	// Want this to point to in-memory database. We'll replay raft log entries
	// to bring db upto speed.
	const sqlOpenPath = "file::memory:?mode=memory&cache=shared"
	return sqlOpenPath
}

// Returns database path to use for the raft log.
func GetSqliteRaftLogPath() string {
	localId := GetLocalNodeId()
	localId = strings.Replace(localId, ":", "-", -1)
	return "./sqlite-raft-log-" + localId
}

// Issues append entries rpc to replicate command to majority of nodes and returns
// true on success.
func IssueAppendEntriesRpcToMajorityNodes(event *RaftClientCommandRpcEvent) bool {

	otherNodes := GetOtherNodes()

	// Make RPCs in parallel but wait for all of them to complete.
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	numOtherNodeSuccessRpcs := int32(0)

	for _, node := range otherNodes {
		// Pass a copy of node to avoid a race condition.
		go func(node pb.RaftClient) {
			defer waitGroup.Done()
			success := IssueAppendEntriesRpcToNode(event.request, node)
			if success {
				atomic.AddInt32(&numOtherNodeSuccessRpcs, 1)
			}
		}(node)
	}

	waitGroup.Wait()

	// +1 to include the copy at the primary as well.
	numReplicatedData:= int64(numOtherNodeSuccessRpcs) + 1
	if numReplicatedData >= GetQuorumSize() {
		return true
	} else {
		return false
	}
}

// Issues an append entries rpc to given raft client and returns true upon success
func IssueAppendEntriesRpcToNode(request pb.ClientCommandRequest, client pb.RaftClient) bool {
	if !IsLeader() {
		return false
	}
	currentTerm := RaftCurrentTerm()
	appendEntryRequest := pb.AppendEntriesRequest{}
	appendEntryRequest.Term = currentTerm
	appendEntryRequest.LeaderId = GetLocalNodeId()
	appendEntryRequest.PrevLogIndex = GetLeaderPreviousLogIndex()
	appendEntryRequest.PrevLogTerm = GetLeaderPreviousLogTerm()
	appendEntryRequest.LeaderCommit = GetCommitIndex()

	newEntry := pb.LogEntry{}
	newEntry.Term = currentTerm
	newEntry.Data = request.Command

	appendEntryRequest.Entries = append(appendEntryRequest.Entries, &newEntry)

	result, err := client.AppendEntries(context.Background(), &appendEntryRequest)
	if err != nil {
		util.Log(util.ERROR, "Error issuing append entry to node: %v err:%v", client, err)
		return false
	}
	if result.ResponseStatus != uint32(codes.OK) {
		util.Log(util.ERROR, "Error issuing append entry to node: %v response code:%v", client, result.ResponseStatus)
		return false
	}
	util.Log(util.INFO, "AppendEntry Response from node: %v response: %v", client, *result)

	if result.Term > RaftCurrentTerm() {
		ChangeToFollowerIfTermStale(result.Term)
		return false
	} else {
		return true
	}
}


func ChangeToFollowerIfTermStale(theirTerm int64) {
	if theirTerm > RaftCurrentTerm() {
		util.Log(util.INFO, "Changing to follower status because term stale")
		ChangeToFollowerStatus()
		SetRaftCurrentTerm(theirTerm)
	}
}


func appendCommandToLocalLog(event *RaftClientCommandRpcEvent) {
	currentTerm := RaftCurrentTerm()
	newLogEntry := pb.LogEntry{
		Data: event.request.Command,
		Term: currentTerm,
	}
	AddPersistentLogEntry(newLogEntry)
}

// Handles request vote rpc.
func handleRequestVoteRpc(event *RaftRequestVoteRpcEvent) {
	result := pb.RequestVoteResponse{}
	currentTerm := RaftCurrentTerm()

	theirTerm := event.request.Term
	if theirTerm > currentTerm {
		ChangeToFollowerStatus()
		SetRaftCurrentTerm(theirTerm)
		currentTerm = theirTerm
	}

	result.Term = currentTerm
	if event.request.Term < currentTerm {
		result.VoteGranted = false
	} else if AlreadyVoted() {
		result.VoteGranted = false
	} else {
		// Only grant vote if candidate log at least uptodate
		// as receivers (Section 5.2; 5.4)
		if GetLastLogTerm() > event.request.LastLogTerm {
			// Node is more up to date, deny vote.
			result.VoteGranted = false
		} else if GetLastLogTerm() < event.request.LastLogTerm {
			// Their term is more current, grant vote
			result.VoteGranted = true
		} else {
			// Terms match. Let's check log index to see who has longer log history.
			if (GetLastLogIndex() > event.request.LastLogIndex) {
				// Node is more current, deny vote
				result.VoteGranted = false
			} else {
				result.VoteGranted = true
			}
		}
		util.Log(util.INFO,  "Grant vote to other server (%v) at term: %v ? %v", event.request.CandidateId, currentTerm, result.VoteGranted)
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

	// And update our leader id if necessary.
	SetLeaderId(event.request.LeaderId)

	result.Success = true
	event.responseChan<- result
}

// Handles append entries rpc.
func handleAppendEntriesRpc(event *RaftAppendEntriesRpcEvent) {

	// Convert to follower status if term in rpc is newer than ours.
	currentTerm := RaftCurrentTerm()
	theirTerm := event.request.Term
	if theirTerm > currentTerm {
		ChangeToFollowerStatus()
		SetRaftCurrentTerm(theirTerm)
		currentTerm = theirTerm
	} else if theirTerm < currentTerm {
		// We want to reply false here as leader term is stale.
		result := pb.AppendEntriesResponse{}
		result.Term = currentTerm
		result.ResponseStatus = uint32(codes.OK)
		result.Success = false
		event.responseChan<- result
		return
	}

	isHeartBeatRpc := len(event.request.Entries) == 0
	if isHeartBeatRpc {
		handleHeartBeatRpc(event)
		return
	}

	// Otherwise process regular append entries rpc (receiver impl).
	if len(event.request.Entries) > 1 {
		util.Log(util.WARN, "Server sent more than one log entry in append entries rpc")
	}

	result:= pb.AppendEntriesResponse{}
	result.Term = currentTerm
	result.ResponseStatus = uint32(codes.OK)

    // Want to reply false if log does not contain entry at prevLogIndex
    // whose term matches prevLogTerm.
    prevLogIndex := event.request.PrevLogIndex
    prevLogTerm := event.request.PrevLogTerm

    raftLog := GetPersistentRaftLog()
    // Note: log index is 1-based, and so is prevLogIndex.
    containsEntryAtPrevLogIndex := len(raftLog) >= prevLogIndex
	if !containsEntryAtPrevLogIndex {
		util.Log(util.INFO, "Rejecting append entries rpc because we don't have previous log entry at index: %v", prevLogIndex)
		result.Success = false
		event.responseChan<- result
		return
	}
	// So, we have an entry at that position. Confirm that the terms match.
	// We want to reply false if the terms do not match at that position.
	prevLogIndexZeroBased := prevLogIndex - 1  // -1 because log index is 1-based.
	ourLogEntry := raftLog[prevLogIndexZeroBased]
	entryTermsMatch := ourLogEntry.LogEntry.Term == prevLogTerm
	if !entryTermsMatch {
		util.Log(util.INFO, "Rejecting append entries rpc because log terms don't match. Ours: %v, theirs: %v", ourLogEntry.LogEntry.Term, prevLogTerm )
		result.Success = false
		event.responseChan<- result
		return
	}

	// Delete log entries that conflict with those from leader.
	newEntry := event.request.Entries[0]
	newLogIndex := prevLogIndex + 1
	newLogIndexZeroBased := newLogIndex -1
	containsEntryAtNewLogIndex := int64(len(raftLog)) >= newLogIndex
	if containsEntryAtNewLogIndex {
		// Check whether we have a conflict (terms differ).
		ourEntry := raftLog[newLogIndexZeroBased]
		theirEntry := newEntry

		haveConflict := ourEntry.LogEntry.Term != theirEntry.Term
		if haveConflict {
			// We must make our logs match the leader. Thus, we need to delete
			// all our entries starting from the new entry position.
			DeletePersistentLogEntryInclusive(newLogIndex)
			AddPersistentLogEntry(*newEntry)
			result.Success = true
		} else {
			// We do not need to add any new entries to our log, because existing
			// one already matches the leader.
			result.Success = true
		}
	} else {
		// We need to insert new entry into the log.
		AddPersistentLogEntry(*newEntry)
		result.Success = true

	}

	// Last thing we do is advance our commit pointer.

	if event.request.LeaderCommit > GetCommitIndex() {
		newCommitIndex := min(event.request.LeaderCommit, newLogIndex)
		MoveCommitIndexTo(newCommitIndex)
	}

	event.responseChan<- result
}

func min(a, b int64) int64 {
	if a < b {
		return a
	} else {
		return b
	}
}

// Returns other nodes client connections
func GetOtherNodes() []pb.RaftClient {
	return raftServer.otherNodes
}


// Increments the number of received votes.
func IncrementVoteCount() {
	atomic.AddInt64(&raftServer.receivedVoteCount, 1)
}

// Returns the index of the last entry in the raft log. Index is 1-based.
func GetLastLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return GetLastLogIndexLocked()
}

func GetLastLogIndexLocked() int64 {
	raftLog := raftServer.raftState.persistentState.log
	lastItem := raftLog[len(raftLog) - 1]

	if lastItem.LogIndex != int64(len(raftLog)) {
		// TODO: Remove this sanity check ...
		log.Fatalf("Mismatch between stored log index value and # of entries. Last stored log index: %v num entries: %v ", lastItem.LogIndex, len(raftLog))
	}
	return lastItem.LogIndex
}

// Returns the term for the last entry in the raft log.
func GetLastLogTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return GetLastLogTermLocked()
}

func GetLastLogTermLocked() int64 {
	raftLog := raftServer.raftState.persistentState.log
	lastItem := raftLog[len(raftLog) - 1]

	return lastItem.LogEntry.Term
}


// Updates raft current term to a new one.
func SetRaftCurrentTerm(term int64) {
	currentTerm := RaftCurrentTerm()
	if term < currentTerm {
		log.Fatalf("Trying to update to the  lesser term: %v current: %v", term, currentTerm)
	} else if term == currentTerm {
		// Concurrent rpcs can lead to duplicated attempts to update terms.
		return
	}

	// Note: Be wary of calling functions that also take the lock as
	// golang locks are not reentrant.
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	SetPersistentCurrentTermLocked(term)

	// Since it's a new term reset who voted for and if heard heartbeat from leader as candidate.
	SetPersistentVotedForLocked("")
	ResetReceivedVoteCount()
	SetReceivedHeartbeatLocked(false)
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
		// Pass a copy of node to avoid a race condition.
		go func(node pb.RaftClient) {
			defer waitGroup.Done()
			RequestVoteFromNode(node)
		}(node)
	}

	waitGroup.Wait()
	util.Log(util.INFO, "Have %v votes at end", GetVoteCount())
	return GetVoteCount()
}

// Requests a vote from the given node.
func RequestVoteFromNode(node pb.RaftClient) {
	if (GetServerState() != Candidate) {
		return
	}

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
	if result.ResponseStatus != uint32(codes.OK) {
		util.Log(util.ERROR, "Error with vote rpc entry to node: %v response code:%v", node, result.ResponseStatus)
		return
	}
	util.Log(util.INFO, "Vote response: %v", *result)
	if result.VoteGranted {
		IncrementVoteCount()
	}
	// Change to follower status if our term is stale.
	if (result.Term > RaftCurrentTerm()) {
		util.Log(util.INFO, "Changing to follower status because term stale")
		ChangeToFollowerStatus()
		SetRaftCurrentTerm(result.Term)
	}
}

// Changes to Follower status.
func ChangeToFollowerStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.serverState = Follower
}

// Converts the node to a leader status from a candidate
func ChangeToLeaderStatus() {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

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
	util.Log(util.INFO, "Starting candidate loop")
	for {
		if GetServerState() != Candidate {
			util.Log(util.INFO, "Stopping candidate loop")
			return
		}
		IncrementElectionTerm()
		util.Log(util.INFO, "Starting new election term: %v", RaftCurrentTerm())
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
		util.Log(util.INFO, "Potential split votes/not enough votes. Performing Randomized wait.")
		timeoutTimer := RandomizedElectionTimeout()
		timeoutDone := false
		for {
			// While processing RPCs below, we may convert from candidate status to follower
			if GetServerState() != Candidate {
				util.Log(util.INFO, "Stopping candidate loop. Exit from inner loop")
				return
			}
			if timeoutDone {
				break
			}
			if GetReceivedHeartbeat() {
				// We have another leader and should convert to follower status.
				util.Log(util.INFO, "Heard from another leader. Converting to follower status")
				ChangeToFollowerStatus()
				return
			}
			select {
			case event := <-raftServer.events:
				handleRpcEvent(event)
			case <-timeoutTimer.C:
				timeoutDone = true
				break

			}
		}
	}
}


// Reinitializes volatile leader state
func ReinitVolatileLeaderState() {
	if !IsLeader() {
		return
	}
	volatileLeaderState := raftServer.raftState.volatileLeaderState

	// Reset match index to 0.
	numOtherNodes := len(GetOtherNodes())
	volatileLeaderState.matchIndex = make([]int64, numOtherNodes)
	for i, _ := range volatileLeaderState.matchIndex {
		volatileLeaderState.matchIndex[i] = 0
	}

	// Reset next index to leader last log index + 1.
	newVal := GetLastLogIndex() + 1
	volatileLeaderState.nextIndex = make([]int64, numOtherNodes)
	for i, _ := range volatileLeaderState.nextIndex {
		volatileLeaderState.nextIndex[i] = newVal
	}
}


func GetServerState() ServerState {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

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
	// - Also change to follower status if term is stale in rpc request/response
	util.Log(util.INFO, "Starting leader loop")
	ReinitVolatileLeaderState()

	// Send heartbeats to followers in the background.
	go func() {
		util.Log(util.INFO, "Starting to Send heartbeats to followers in background")
		for {
			if GetServerState() != Leader {
				util.Log(util.INFO, "No longer leader. Stopping heartbeat rpcs")
				return
			}
			SendHeartBeatsToFollowers()
			time.Sleep(time.Duration(GetHeartbeatIntervalMillis()) * time.Millisecond)
		}
	}()

	for {
		// While processing RPC, we may learn we no longer a valid leader.
		if GetServerState() != Leader {
			util.Log(util.INFO, "Stopping leader loop")
			return
		}
		select {
		case event := <-raftServer.events:
			handleRpcEvent(event)
		}
	}
}

// Send heart beat rpcs to followers in parallel and waits for them to all complete.
func SendHeartBeatsToFollowers() {
	otherNodes := GetOtherNodes()

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(otherNodes))

	for _,node := range otherNodes {
		// Send RPCs in parallel. Pass copy of node to avoid race conditions.
		go func(node pb.RaftClient) {
			defer waitGroup.Done()
			SendHeartBeatRpc(node)
		}(node)
	}
	waitGroup.Wait()
}


// Sends a heartbeat rpc to the given raft node.
func SendHeartBeatRpc(node pb.RaftClient) {
	request := pb.AppendEntriesRequest{}
	request.Term = RaftCurrentTerm()
	request.LeaderId = GetLocalNodeId()
	request.LeaderCommit = GetCommitIndex()

	// Log entries are empty/nil for heartbeat rpcs, so no need to
	// set previous log index, previous log term.
	request.Entries = nil

	result, err := node.AppendEntries(context.Background(), &request)
	if err != nil {
		util.Log(util.ERROR, "Error sending hearbeat to node: %v Error: %v", node, err)
		return
	}
	util.Log(util.INFO, "Heartbeat RPC Response from node: %v Response: %v", node, *result)
}

// PrevLogTerm value  used in the appendentries rpc request. Should be called _after_ local local updated.
func GetLeaderPreviousLogTerm() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	// Because we would have just stored a new entry to our local log, when this
	// method is called, the previous entry is the one before that.
	raftLog := raftServer.raftState.persistentState.log
	lastEntryIndex := len(raftLog)-1
	previousEntryIndex := lastEntryIndex - 1
	previousEntry := raftLog[previousEntryIndex]
	return previousEntry.LogEntry.Term
}

// PrevLogIndex value  used in the appendentries rpc request. Should be called _after_ local log
// already updated.
func GetLeaderPreviousLogIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	// Because we would have just stored a new entry to our local log, when this
	// method is called, the previous entry is the one before that.
	raftLog := raftServer.raftState.persistentState.log
	lastEntryIndex := len(raftLog)-1
	previousEntryIndex := lastEntryIndex - 1
	previousEntry := raftLog[previousEntryIndex]
	return previousEntry.LogIndex
}

// Leader commit value used in the appendentries rpc request.
func GetCommitIndex() int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftState.volatileState.commitIndex
}

func GetLastApplied() int64  {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	return raftServer.raftState.volatileState.lastApplied
}

func SetLastApplied(newValue int64) int64 {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileState.lastApplied = newValue
}

func SetCommitIndex(newValue int64) {
	raftServer.lock.Lock()
	defer raftServer.lock.Unlock()

	raftServer.raftState.volatileState.commitIndex = newValue
}


// Overall loop for the server.
func StartServerLoop() {

	for {
		serverState := GetServerState()
		if (serverState == Leader) {
			LeaderLoop()
		} else if (serverState == Follower) {
			FollowerLoop()
		} else if (serverState == Candidate) {
			CandidateLoop()
		} else {
			log.Fatalf("Unexpected / unknown server state: %v", serverState)
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
