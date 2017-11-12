# ReSqlite

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