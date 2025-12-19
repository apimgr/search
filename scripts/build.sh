#!/bin/bash
# Build script for Search metasearch engine
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Search Build Script ===${NC}"

# Configuration
VERSION=${VERSION:-$(cat release.txt 2>/dev/null || echo "dev")}
BUILD_DIR="binaries"
RELEASE_DIR="releases"
PLATFORMS=${PLATFORMS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"}
USE_DOCKER=${USE_DOCKER:-true}

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

# Create directories
mkdir -p "$BUILD_DIR"
mkdir -p "$RELEASE_DIR"

# Build function
build_binary() {
    local goos=$1
    local goarch=$2
    local output_name="search"
    
    if [ "$goos" = "windows" ]; then
        output_name="search.exe"
    fi
    
    local output_path="$BUILD_DIR/${goos}_${goarch}/$output_name"
    mkdir -p "$BUILD_DIR/${goos}_${goarch}"
    
    print_info "Building for $goos/$goarch..."
    
    if [ "$USE_DOCKER" = "true" ]; then
        docker run --rm \
            -v "$(pwd):/app" \
            -w /app \
            -e GOOS="$goos" \
            -e GOARCH="$goarch" \
            -e CGO_ENABLED=0 \
            golang:latest \
            go build -ldflags "-X main.Version=$VERSION -s -w" \
            -o "$output_path" \
            src/main.go
    else
        GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
            go build -ldflags "-X main.Version=$VERSION -s -w" \
            -o "$output_path" \
            src/main.go
    fi
    
    if [ $? -eq 0 ]; then
        print_info "✓ Built: $output_path"
    else
        print_error "✗ Failed to build for $goos/$goarch"
        return 1
    fi
}

# Parse command line arguments
BUILD_ALL=false
BUILD_CURRENT=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --all)
            BUILD_ALL=true
            shift
            ;;
        --current)
            BUILD_CURRENT=true
            shift
            ;;
        --no-docker)
            USE_DOCKER=false
            shift
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--all] [--current] [--no-docker] [--version VERSION]"
            exit 1
            ;;
    esac
done

# Detect current platform
CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m)

case $CURRENT_ARCH in
    x86_64)
        CURRENT_ARCH="amd64"
        ;;
    aarch64|arm64)
        CURRENT_ARCH="arm64"
        ;;
esac

if [ "$CURRENT_OS" = "darwin" ]; then
    CURRENT_OS="darwin"
elif [ "$CURRENT_OS" = "linux" ]; then
    CURRENT_OS="linux"
fi

print_info "Version: $VERSION"
print_info "Build directory: $BUILD_DIR"
print_info "Using Docker: $USE_DOCKER"

# Build based on flags
if [ "$BUILD_ALL" = "true" ]; then
    print_info "Building for all platforms..."
    for platform in $PLATFORMS; do
        IFS='/' read -r goos goarch <<< "$platform"
        build_binary "$goos" "$goarch"
    done
elif [ "$BUILD_CURRENT" = "true" ]; then
    print_info "Building for current platform ($CURRENT_OS/$CURRENT_ARCH)..."
    build_binary "$CURRENT_OS" "$CURRENT_ARCH"
else
    # Default: build for current platform
    print_info "Building for current platform ($CURRENT_OS/$CURRENT_ARCH)..."
    print_info "Use --all to build for all platforms, --current for explicit current platform"
    build_binary "$CURRENT_OS" "$CURRENT_ARCH"
fi

echo -e "${GREEN}=== Build Complete ===${NC}"
