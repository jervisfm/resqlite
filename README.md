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

You would also need an implementation of golang sqlite3 drvier. The one we used is go-sqlite3 - https://github.com/mattn/go-sqlite3

Install it like so on OS X:
```
$ brew install sqlite3
$ go get github.com/mattn/go-sqlite3
$ go install github.com/mattn/go-sqlite3
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

### Pending Work items:
Main item left is implementing cluster replication. To get there we need to:

* Add methods to accessing raft persistent state. 
    - Have it backup to in memory state to start: DONE.
    - Migrate it later to real durable storage later: DONE
* Add a method to raft service to receive client command: DONE
* Add sqlite dependency: DONE
* Implement AppendEntries receiver.
* Restore replicated state machine upon server start app.
* Apply sql command.


Performance work:
* Benchmark single node performance (w/o replication overhead)
* Benchmark replicated cluster performance (3 nodes).

### Testing

#### Polyglot RPC Testing

Using polygot for rpc testing: https://github.com/grpc-ecosystem/polyglot/releases/tag/v1.5.0
Create test students table and add a value to it.
```
$ echo "{ command: 'create table if not exists students(id integer primary key not null, name text); insert or replace into students(id, name) values(1, \"John\")' }" | java -jar ~/bin/polyglot.jar  --command=call --endpoint=localhost:50050 --full_method=proto_raft.Raft/ClientCommand
```

RPC command to query it
```
$ $ echo "{ query: 'select * from students' }" | java -jar ~/bin/polyglot.jar  --command=call --endpoint=localhost:50050 --full_method=proto_raft.Raft/ClientCommand
```

#### REPL based testing

While our repl is still very much a work in progress, it can be used to test manual commands
and responds to a subset of sqlite3 CLI syntax, with all non-deterministic operations removed
(Transactions, Random, Now, etc).

Launch the repl from the resqlite folder with:
```
go run main.go
```


The repl can be used in batch mode to pre-load a number of commands from a file before
awaiting user input:
```
 go run main.go -batch='../data/chinook.txt' -interactive=true
```

### Benchmarking

We use our repl to benchmark performance of our resqlite implementation against sqlite3's cli
(we make the large assumption that CLI overhead is the same and the results will be
representative of underlying db performance).

Note: this is not quite apples to apples yet given that we are using an in-memory db for
ReSqlite; however there are disk IOs since the logs are stored on disk.

We do so by launching the repl in non-interactive mode:
```
go run main.go -batch='../data/chinook.txt'
```

We chose four tests to run our benchmarking on:

         File                   |        Description        
    data/benchmarks/wOnly.db       ~15,000 consecutive writes
    data/benchmarks/rOnly.db       ~15,000 consecutive reads
    data/benchmarks/wHeavy.db      ~1,500 writes, 200 reads, repeated 3 times.
    data/benchmarks/rHeavy.db      200 writes, ~1,500 reads, repeated 3 times.

Running these tests on a single-node cluster (taking median of 3 runs), we find:

    Test         |      DB      |                    Time
    wOnly            ReSqlite          2.52s user 1.58s system 51% cpu 8.019 total
    wOnly            Sqlite3           0.21s user 0.01s system 95% cpu 0.227 total
    rOnly            ReSqlite          3.82s user 3.19s system 24% cpu 28.076 total
    rOnly            Sqlite3           15.92s user 5.88s system 23% cpu 1:34.64 total (of which 1:22:25 from cat)
    wHeavy           ReSqlite          1.45s user 0.85s system 55% cpu 4.136 total
    wHeavy           Sqlite3           0.51s user 0.22s system 19% cpu 3.675 total
    rHeavy           ReSqlite          1.41s user 0.81s system 51% cpu 4.341 total
    rHeavy           Sqlite3           0.79s user 0.33s system 19% cpu 5.792 total

Note: wOnly must be run before rOnly.


### Debugging Issues

* Leader election
    - Yay, seems to be working now.
    - Want to fix though burning CPU on follower loop: Fixed
    - Need to look a go routine leak. We die when running with -race detector: FIXED

* Databases
    - File paths do not seem to be created properly: FIXED
         - Just execute a db statement to get the file to be created.

* Client Command not being processed: FIXED
Issue was that we're not handling case of an unexpected request format.
```
echo "{  }" | java -jar ~/bin/polyglot.jar  --command=call --endpoint=localhost:50050 --full_method=proto_raft.Raft/ClientCommand
```