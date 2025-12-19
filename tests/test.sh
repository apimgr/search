#!/bin/bash
# Test script using Docker golang:alpine
set -e

docker run --rm \
  -v "$(pwd)":/workspace \
  -w /workspace \
  golang:alpine \
  sh -c "go test -v ./src/..."

echo "Tests complete"
