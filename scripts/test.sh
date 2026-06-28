#!/bin/bash
# Test script for Search metasearch engine
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Search Test Suite ===${NC}"

# Configuration
USE_DOCKER=${USE_DOCKER:-true}
COVERAGE=${COVERAGE:-false}
VERBOSE=${VERBOSE:-false}

# Functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-docker)
            USE_DOCKER=false
            shift
            ;;
        --coverage)
            COVERAGE=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--no-docker] [--coverage] [--verbose|-v]"
            exit 1
            ;;
    esac
done

print_info "Using Docker: $USE_DOCKER"
print_info "Coverage: $COVERAGE"
print_info "Verbose: $VERBOSE"

# Coverage output always goes to /tmp/apimgr/search-XXXXXX/ — never to the project tree
PROJECTORG="apimgr"
PROJECTNAME="search"
mkdir -p "/tmp/$PROJECTORG"
COVDIR=$(mktemp -d "/tmp/$PROJECTORG/$PROJECTNAME-XXXXXX")

# Prepare test command
TEST_CMD="go test ./..."
TEST_ARGS=""

if [ "$VERBOSE" = "true" ]; then
    TEST_ARGS="$TEST_ARGS -v"
fi

if [ "$COVERAGE" = "true" ]; then
    TEST_ARGS="$TEST_ARGS -coverprofile=/tmp/covout/coverage.out -covermode=atomic"
fi

# Run tests
if [ "$USE_DOCKER" = "true" ]; then
    print_info "Running tests in Docker..."
    docker run --rm \
        -v "$PWD:/app" \
        -v "$COVDIR:/tmp/covout" \
        -w /app \
        -e CGO_ENABLED=0 \
        -e GOFLAGS=-buildvcs=false \
        casjaysdev/go:latest \
        sh -c "$TEST_CMD $TEST_ARGS"
else
    print_info "Running tests locally..."
    eval "CGO_ENABLED=0 $TEST_CMD $TEST_ARGS"
fi

TEST_EXIT_CODE=$?

# Handle coverage report
if [ "$COVERAGE" = "true" ] && [ $TEST_EXIT_CODE -eq 0 ]; then
    print_info "Generating coverage report..."
    if [ "$USE_DOCKER" = "true" ]; then
        docker run --rm \
            -v "$COVDIR:/tmp/covout" \
            -e CGO_ENABLED=0 \
            casjaysdev/go:latest \
            go tool cover -func=/tmp/covout/coverage.out
    else
        go tool cover -func="$COVDIR/coverage.out"
    fi

    print_info "Coverage report saved to $COVDIR/coverage.out"
fi

# Exit with test result
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}=== All Tests Passed ===${NC}"
    exit 0
else
    echo -e "${RED}=== Tests Failed ===${NC}"
    exit 1
fi
