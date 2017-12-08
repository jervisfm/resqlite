# ReSqlite: Replicated Sqlite

## Introduction
ReSqlite is an extension of Sqlite that aims to add basic replication functionality to Sqlite database.


## Collaborators
* Henri
* Jervis

## Overview
The goal of this project is to add additional resiliency to Sqlite by supporting database replication
on multiple nodes in a consistent manner. 

At the high level the project aims to implement a basic version of the RAFT protocol and leverage that 
protocol to add replication to Sqlite. 

## Raft Implementation

For educational purposes, we will implement our own RAFT protocol. However, given the timing constraints, 
the RAFT implementation will be simple in nature. The key functionality that we want is reliable log replication. 
Notably, we do not intend to support viewchange support in the v0 and the clusters we'd be working with are 
assumed to be fixed in size. 

## Sqlite Replication
The replication of the Sqlite is going to be done at the statement level. That is, we will be replicating 
raw SQL statements and using that to ensure all the nodes arrive at the same end state. Note that this means
SQL statements need to be deterministic and cannot make use of non-deterministic functions such as random().

## Project Setup / Dependencies
For implementation, we make use of Go programming language. Please visit https://golang.org/dl/ to get the lastest
version of Goland. This project was created with Go lang 1.9

Also Install dep tool for Golang dependency management.

```
$ go get -u github.com/golang/dep/cmd/dep
```

For RPC setup, we make use of the gRPC library from Google.

A prerequiste is that you would need to get Protocol Buffers compiler setup. Grab one for your platform from https://github.com/google/protobuf/releases
For OS X, we used https://github.com/google/protobuf/releases/download/v3.4.0/protoc-3.4.0-osx-x86_64.zip. Add protoc to your
environment $PATH.

Install the Protocol Buffer compiler plugin for golang:
```
$ go get -u github.com/golang/protobuf/protoc-gen-go
```

This puts the tool under golang/bin so make sure that's part for your $PATH.
```
$ export PATH=$PATH:$GOPATH/bin
```

### Raft Service
Our simplify ease of testing, our cluster  would consist of 3 nodes such that we can tolerate any single node failture.

From the Raft Paper, we need to implement two RPCS:
* RequestVotes
* AppendEntries

We refer the reader to published page 308 of the RAFT paper (http://www.scs.stanford.edu/17au-cs244b/sched/readings/raft.pdf)
which summarizes the semantics of these RPCs as well as what state (both persistent and volatile) needs to be maintained on each node.

We will implement these two RPCs using Google gRPCs under a RaftService declaration.

### ReSqlite Service
We will also have another service for interacting with Sqlite. The idea of this service is to take a SQL command as a request to
be executed on a sqlite instance and then return the result of that execution back together. 

The motivation for decoupling the Sqlite service from the Raft service is have clean separation of responsibilities between the
services. The Raft protocol does not care per-se what the contents of the replicated log are.

### Implementation Plan. 
The following is a proposed implementation structure for the project:

* raft/raft.go: Implements main raft replication business logic as a library
    - To keep logic simple, we would use an event-based approach for the main loop.
* raft-cli/main.go (server implementation binary that can act as a basic raft node)
    - To test the raft implementation we can start from scractch and use a constant set of data to replicate.

* server/resqlite.go (Library that leverages Raft to replicate sqlite statements and implement replicated sqlite service)
* server/main.go (server binary that nodes runs). This involves:
    - Replicate new entry to other nodes.
    - Add it locally on success and execute
    - Reply to client with result.

* client/resqlite.go (library)
* client/main.go (client binary) <- Perhaps this can be replicaed with polyglot (https://github.com/grpc-ecosystem/polyglot#server-reflection)

### Debugging Issues

* Leader election
    - Yay, seems to be working now.
    - Want to fix though burning CPU on follower loop: Fixed
    - Need to look a go routine leak. We die when running with -race detector. This happens to followers
```
2017-11-26 23:37:34.989 [INFO] Have 1 votes at end
2017-11-26 23:37:34.989 [INFO] Potential split votes/not enough votes. Performing Randomized wait.
2017-11-26 23:37:35.337 [INFO] Grant vote to other server at term: 24
2017-11-26 23:37:35.337 [INFO] Stopping candidate loop. Exit from inner loop
2017-11-26 23:37:35.337 [INFO] Starting  follower loop
2017-11-26 23:37:41.814 [INFO] Grant vote to other server at term: 25
race: limit on 8192 simultaneously alive goroutines is exceeded, dying
exit status 66
```

After adding more logging, it seems that we're dying after handling 8000 RPCs:

```
2017-12-07 20:30:23.953 [INFO] Processing rpc #8092 event: {{<nil> 0xc421a74280}}
2017-12-07 20:30:23.968 [INFO] Processing rpc #8093 event: {{<nil> 0xc421a4c6e0}}
2017-12-07 20:30:23.983 [INFO] Processing rpc #8094 event: {{<nil> 0xc421a216d0}}
2017-12-07 20:30:24.000 [INFO] Processing rpc #8095 event: {{<nil> 0xc421a744b0}}
2017-12-07 20:30:24.016 [INFO] Processing rpc #8096 event: {{<nil> 0xc421a745a0}}
2017-12-07 20:30:24.031 [INFO] Processing rpc #8097 event: {{<nil> 0xc421a726e0}}
race: limit on 8192 simultaneously alive goroutines is exceeded, dying
```

This means that for some reason go routine processing RPC is staying around and not going away.

Okay, the leak looks to be coming from the timeout wait channel:

```
832 @ 0x105bedd 0x105bfbe 0x102ddfb 0x102db53 0x15a247d 0x108a721
#       0x15a247c       github.com/jervisfm/resqlite/raft.GetTimeoutWaitChannel.func1+0x5c      /Users/jmuindi/code/personal/go/src/github.com/jervisfm/resqlite/raft/raft.go:249

14 @ 0x105bedd 0x105bfbe 0x107b593 0x15a2460 0x108a721
#       0x107b592       time.Sleep+0x132                                                        /usr/local/go/src/runtime/time.go:65
#       0x15a245f       github.com/jervisfm/resqlite/raft.GetTimeoutWaitChannel.func1+0x3f      /Users/jmuindi/code/personal/go/src/github.com/jervisfm/resqlite/raft/raft.go:248

```