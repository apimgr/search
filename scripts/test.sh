#!/bin/bash
# Test script for Search metasearch engine
set -e

# Colors for output
TEST_RED='\033[0;31m'
TEST_GREEN='\033[0;32m'
TEST_YELLOW='\033[1;33m'
# No Color
TEST_NC='\033[0m'

echo -e "${TEST_GREEN}=== Search Test Suite ===${TEST_NC}"

# Functions
__print_info() {
    echo -e "${TEST_GREEN}[INFO]${TEST_NC} $1"
}

__print_warn() {
    echo -e "${TEST_YELLOW}[WARN]${TEST_NC} $1"
}

__print_error() {
    echo -e "${TEST_RED}[ERROR]${TEST_NC} $1"
}

TEST_SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
TEST_PROJECT_DIR=$(cd -- "$TEST_SCRIPT_DIR/.." && pwd)

# Parse command line arguments (kept for backward-compatible invocation)
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-docker)
            __print_warn "--no-docker is unsupported — make test always runs in Docker per project rules"
            shift
            ;;
        --coverage)
            # make test always runs with coverage
            shift
            ;;
        --verbose|-v)
            __print_warn "--verbose is unsupported — make test already runs go test -v"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--coverage] [--verbose|-v]"
            exit 1
            ;;
    esac
done

__print_info "Delegating to 'make test' (single source of truth for the test pipeline)..."
make -C "$TEST_PROJECT_DIR" test

echo -e "${TEST_GREEN}=== All Tests Passed ===${TEST_NC}"
