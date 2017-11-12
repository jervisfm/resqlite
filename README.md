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
For implementation, we make use of Go programming language.

For RPC setup, we make use of the gRPC library from Google.
