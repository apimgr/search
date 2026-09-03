#!/bin/bash
# Release script for Search metasearch engine
set -e

# Colors for output
RELEASE_RED='\033[0;31m'
RELEASE_GREEN='\033[0;32m'
RELEASE_YELLOW='\033[1;33m'
RELEASE_BLUE='\033[0;34m'
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
    echo "Usage: $0 <version> [--skip-tests] [--skip-build]"
    echo "Example: $0 v0.2.0"
    exit 1
fi

VERSION=$1
RELEASE_SKIP_TESTS=false
RELEASE_SKIP_BUILD=false

# Parse additional arguments
shift
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-tests)
            RELEASE_SKIP_TESTS=true
            shift
            ;;
        --skip-build)
            RELEASE_SKIP_BUILD=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate version format
if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    __print_error "Invalid version format. Expected: vX.Y.Z (e.g., v0.2.0)"
    exit 1
fi

__print_info "Preparing release $VERSION"

# Check if release already exists
if [ -d "releases/$VERSION" ]; then
    __print_error "Release $VERSION already exists"
    exit 1
fi

# Run tests unless skipped
if [ "$RELEASE_SKIP_TESTS" = "false" ]; then
    __print_info "Running tests..."
    if ! make -C "$RELEASE_PROJECT_DIR" test; then
        __print_error "Tests failed. Release aborted."
        exit 1
    fi
    __print_info "✓ Tests passed"
else
    __print_warn "Skipping tests"
fi

# Build binaries unless skipped
if [ "$RELEASE_SKIP_BUILD" = "false" ]; then
    __print_info "Building binaries for all platforms..."
    if ! VERSION=$VERSION make -C "$RELEASE_PROJECT_DIR" build; then
        __print_error "Build failed. Release aborted."
        exit 1
    fi
    __print_info "✓ Build completed"
else
    __print_warn "Skipping build"
fi

# Create release directory
RELEASE_DIR="releases/$VERSION"
mkdir -p "$RELEASE_DIR"

__print_info "Creating release archives..."

# Create archives for each platform
for platform_dir in binaries/*/; do
    if [ -d "$platform_dir" ]; then
        platform=${platform_dir%/}
        platform=${platform##*/}
        __print_info "Packaging $platform..."
        
        # Create archive
        cd "binaries/$platform"
        if [[ $platform == windows* ]]; then
            zip -q "../../$RELEASE_DIR/search-$VERSION-$platform.zip" *
        else
            tar czf "../../$RELEASE_DIR/search-$VERSION-$platform.tar.gz" *
        fi
        cd ../..
        
        # Generate checksums
        if [[ $platform == windows* ]]; then
            sha256sum "$RELEASE_DIR/search-$VERSION-$platform.zip" >> "$RELEASE_DIR/checksums.txt"
        else
            sha256sum "$RELEASE_DIR/search-$VERSION-$platform.tar.gz" >> "$RELEASE_DIR/checksums.txt"
        fi
    fi
done

# Update release.txt
echo "$VERSION" > release.txt
__print_info "Updated release.txt to $VERSION"

# Create release notes template
cat > "$RELEASE_DIR/RELEASE_NOTES.md" << EOF
# Search $VERSION

## Release Date
$(date +%Y-%m-%d)

## Changes

### Added
- 

### Changed
- 

### Fixed
- 

### Security
- 

## Installation

Download the appropriate archive for your platform:

\`\`\`bash
# Linux (amd64)
wget https://github.com/apimgr/search/releases/download/$VERSION/search-$VERSION-linux_amd64.tar.gz
tar xzf search-$VERSION-linux_amd64.tar.gz
./search

# macOS (amd64)
wget https://github.com/apimgr/search/releases/download/$VERSION/search-$VERSION-darwin_amd64.tar.gz
tar xzf search-$VERSION-darwin_amd64.tar.gz
./search

# Windows (amd64)
# Download search-$VERSION-windows_amd64.zip and extract
\`\`\`

## Checksums

See [checksums.txt](./checksums.txt) for SHA-256 checksums of all release files.

## Docker

\`\`\`bash
docker pull ghcr.io/apimgr/search:$VERSION
docker run -p 64580:80 ghcr.io/apimgr/search:$VERSION
\`\`\`

## Full Changelog

See [CHANGELOG.md](../../CHANGELOG.md) for detailed changes.
EOF

__print_info "Created release notes template at $RELEASE_DIR/RELEASE_NOTES.md"

# Summary
echo ""
echo -e "${RELEASE_BLUE}=== Release Summary ===${RELEASE_NC}"
echo -e "${RELEASE_GREEN}Version:${RELEASE_NC} $VERSION"
echo -e "${RELEASE_GREEN}Release directory:${RELEASE_NC} $RELEASE_DIR"
echo -e "${RELEASE_GREEN}Archives created:${RELEASE_NC}"
ls -lh "$RELEASE_DIR"/*.{tar.gz,zip} 2>/dev/null || true
echo ""
echo -e "${RELEASE_YELLOW}Next steps:${RELEASE_NC}"
echo "1. Edit $RELEASE_DIR/RELEASE_NOTES.md with actual changes"
echo "2. Update CHANGELOG.md"
echo "3. Commit changes: git add . && git commit -m 'Release $VERSION'"
echo "4. Create tag: git tag -a $VERSION -m 'Release $VERSION'"
echo "5. Push changes: git push && git push --tags"
echo "6. Create GitHub release with archives from $RELEASE_DIR"
echo ""
echo -e "${RELEASE_GREEN}=== Release $VERSION Prepared ===${RELEASE_NC}"
