//go:generate protoc -I ../proto/raft --go_out=plugins=grpc:../proto/raft ../proto/raft/raft.proto


// Contains general utility code.
package util

import (

	//"math/bits"
	"fmt"
	"time"
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

// Logs a message if debugging upto to given level is enabled.
// debugLevel: One of {ERROR, WARN, INFO, VERBOSE, EXTRA_VERBOSE}
// format: format string
// v : optional list of formatted parameters in format string.
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
	fmt.Print(time.Now().Format("2006-01-02 15:04:05.000"))
	if (debugLevel == ERROR) {
		// Red color
		fmt.Print("\x1b[31m")
		fmt.Print(" [ERROR] ")
		fmt.Print("\x1b[0m")
	} else if (debugLevel == WARN) {
		// Yellow Color
		fmt.Print("\x1b[33m")
		fmt.Print(" [WARNING] ")
		fmt.Print("\x1b[0m")
	} else if (debugLevel == INFO) {
		// Green Color
		fmt.Print("\x1b[32m")
		fmt.Print(" [INFO] ")
		fmt.Print("\x1b[0m")
	} else if (debugLevel == VERBOSE) {
		// Blue Color
		fmt.Print("\x1b[34m")
		fmt.Print(" [VERBOSE] ")
		fmt.Print("\x1b[0m")
	} else if (debugLevel == EXTRA_VERBOSE) {
		// Magneta Color
		fmt.Print("\x1b[35m")
		fmt.Print(" [EXTRA VERBOSE] ")
		fmt.Print("\x1b[0m")
	}
	fmt.Printf(format, v...)
	fmt.Print("\n")
}