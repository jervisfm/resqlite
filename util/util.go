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

func Log(debugLevel int, format string, v ...interface{}) {
	if debugLevel < 0 {
		return
	}

	shouldLog := debugLevel <= DebugLevel
	if !shouldLog {
		return
	}

	// For improved readability, color the logging messages.
	// See https://en.wikipedia.org/wiki/ANSI_escape_code#Colors for ANSI color codes.

	if (debugLevel == ERROR) {
		// Red color
		log.Print("\x1b[31m")
		log.Print("[ERROR] ")
		log.Print("\x1b[0m")
	} else if (debugLevel == WARN) {
		// Yellow Color
		log.Print("\x1b[33m")
		log.Print("[WARNING] ")
		log.Print("\x1b[0m")
	} else if (debugLevel == INFO) {
		// Green Color
		log.Print("\x1b[32m")
		log.Print("[INFO] ")
		log.Print("\x1b[0m")
	} else if (debugLevel == VERBOSE) {
		// Blue Color
		log.Print("\x1b[34m")
		log.Print("[VERBOSE] ")
		log.Print("\x1b[0m")
	} else if (debugLevel == EXTRA_VERBOSE) {
		// Magneta Color
		log.Print("\x1b[35m")
		log.Print("[EXTRA VERBOSE] ")
		log.Print("\x1b[0m")
	}
	log.Printf(format, v...)
}