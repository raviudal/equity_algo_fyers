#!/bin/bash
set -e

echo "=========================================================="
echo " Building AlgoEngine for GCP Linux (e2-micro amd64)"
echo "=========================================================="

# Cross-compile for GCP Linux x86_64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o algoengine main.go

echo "Build successful! Created binary: algoengine"
ls -lh algoengine
