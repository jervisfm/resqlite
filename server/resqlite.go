package server

import (
    // pb "github.com/jervisfm/resqlite/proto/resqlite"
    "os/exec"
)

const (
    modifier = "SELECT "
)
var db string

// Cache db identity for all future commands made through this session
func CacheDb(inputtedDb string) {
    db = inputtedDb
}

// Parse command to determine whether it is RO or making changes
func ParseCommand(query string) bool {
    // input trimmed at client
    if (len(query) >= len(modifier) && query[:len(modifier)] == modifier) {
        return false;
    }
    return true;
}

// Called by client to execute command
func ExecCommand(query string) (string, error) {
    
    readOnly := ParseCommand(query)
    var out []byte
    var err error

    if readOnly {
        // execute locally
        out, err = exec.Command("sh", "-c", db + " \"" + query + "\"").Output()
    }

    return string(out), err
}