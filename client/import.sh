#!/usr/bin/env sh

echo "Running SQL data import ..."
go run main.go -batch='../data/chinook.txt' --interactive=true