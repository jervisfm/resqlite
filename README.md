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
    - Need to look a go routine leak. We die when running with -race detector: FIXED