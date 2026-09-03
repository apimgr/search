#!/bin/bash
# Build script for Search metasearch engine
set -e

# Colors for output
BUILD_RED='\033[0;31m'
BUILD_GREEN='\033[0;32m'
BUILD_YELLOW='\033[1;33m'
# No Color
BUILD_NC='\033[0m'

echo -e "${BUILD_GREEN}=== Search Build Script ===${BUILD_NC}"

# Functions
__print_info() {
    echo -e "${BUILD_GREEN}[INFO]${BUILD_NC} $1"
}

__print_warn() {
    echo -e "${BUILD_YELLOW}[WARN]${BUILD_NC} $1"
}

__print_error() {
    echo -e "${BUILD_RED}[ERROR]${BUILD_NC} $1"
}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
BUILD_PROJECT_DIR=$(cd -- "$SCRIPT_DIR/.." && pwd)

# Parse command line arguments (kept for backward-compatible invocation)
BUILD_ARM64=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --all|--current)
            # make build already builds every target platform in one pass
            shift
            ;;
        --arm64)
            BUILD_ARM64=true
            shift
            ;;
        --no-docker)
            __print_warn "--no-docker is unsupported — make build always runs in Docker per project rules"
            shift
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--all] [--arm64] [--version VERSION]"
            exit 1
            ;;
    esac
done

export VERSION

__print_info "Delegating to 'make build' (single source of truth for the build pipeline)..."
make -C "$BUILD_PROJECT_DIR" build

if [ "$BUILD_ARM64" = "true" ]; then
    __print_info "Delegating to 'make build-arm64'..."
    make -C "$BUILD_PROJECT_DIR" build-arm64
fi

echo -e "${BUILD_GREEN}=== Build Complete ===${BUILD_NC}"
