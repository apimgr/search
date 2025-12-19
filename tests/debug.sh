#!/bin/bash
# Debug script - interactive shell with golang:alpine
set -e

docker run --rm -it \
  -v "$(pwd)":/workspace \
  -w /workspace \
  -p 8080:8080 \
  golang:alpine \
  sh
