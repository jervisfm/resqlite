//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto


// Contains general utility code.
package util

import (
	"log"
	//"math/bits"
)


// Debug log levels
const (
	ERROR = iota
	WARN
	INFO
	VERBOSE
	EXTRA_VERBOSE
)

// Only output to INFO level.
var DebugLevel int = INFO
