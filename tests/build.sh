#!/bin/bash
# Build script using Docker golang:alpine
set -e

docker run --rm \
  -v "$(pwd)":/workspace \
  -w /workspace \
  golang:alpine \
  sh -c "go build -o binaries/search ./src"

echo "Build complete: binaries/search"
