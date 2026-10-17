#!/bin/bash
# Release script for Search metasearch engine
set -eo pipefail

# Colors for output
RELEASE_RED='\033[0;31m'
RELEASE_GREEN='\033[0;32m'
RELEASE_YELLOW='\033[1;33m'
# No Color
RELEASE_NC='\033[0m'

echo -e "${RELEASE_GREEN}=== Search Release Script ===${RELEASE_NC}"

# Functions
__print_info() {
    echo -e "${RELEASE_GREEN}[INFO]${RELEASE_NC} $1"
}

__print_warn() {
    echo -e "${RELEASE_YELLOW}[WARN]${RELEASE_NC} $1"
}

__print_error() {
    echo -e "${RELEASE_RED}[ERROR]${RELEASE_NC} $1"
}

RELEASE_SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
RELEASE_PROJECT_DIR=$(cd -- "$RELEASE_SCRIPT_DIR/.." && pwd)

# Check if version is provided
if [ -z "$1" ]; then
    __print_error "Version number required"
    echo "Usage: $0 <version> [--skip-tests]"
    echo "Example: $0 0.2.0"
    exit 1
fi

VERSION="$1"
RELEASE_SKIP_TESTS=false

# Parse additional arguments
shift
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-tests)
            RELEASE_SKIP_TESTS=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Version in release.txt is bare semver (no v prefix); the v prefix is added
# to the git tag by the Makefile per AI.md PART 25
if [[ ! $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    __print_error "Invalid version format. Expected: X.Y.Z (e.g., 0.2.0)"
    exit 1
fi

__print_info "Preparing release $VERSION"

# release.txt is the version source of truth read by the Makefile
echo "$VERSION" >"$RELEASE_PROJECT_DIR/release.txt"
__print_info "Updated release.txt to $VERSION"

# Run tests unless skipped
if [ "$RELEASE_SKIP_TESTS" = "false" ]; then
    __print_info "Running tests..."
    if ! make -C "$RELEASE_PROJECT_DIR" test; then
        __print_error "Tests failed. Release aborted."
        exit 1
    fi
    __print_info "Tests passed"
else
    __print_warn "Skipping tests"
fi

# make release is the single source of truth for build, checksums, source
# archive and the GitHub release itself (per AI.md PART 25)
__print_info "Delegating to 'make release'..."
make -C "$RELEASE_PROJECT_DIR" release

echo -e "${RELEASE_GREEN}=== Release $VERSION Complete ===${RELEASE_NC}"
