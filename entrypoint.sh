#!/bin/bash

cd /summarizer

# Build the Go application
go build -o summarizer_server cmd/server/main.go

# Run the server and redirect stdout and stderr to a log file
./summarizer_server >> /var/log/summarizer/summarizer.log 2>&1
