#!/bin/bash
# Release script for Search metasearch engine
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Search Release Script ===${NC}"

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

# Check if version is provided
if [ -z "$1" ]; then
    print_error "Version number required"
    echo "Usage: $0 <version> [--skip-tests] [--skip-build]"
    echo "Example: $0 v0.2.0"
    exit 1
fi

VERSION=$1
SKIP_TESTS=false
SKIP_BUILD=false

# Parse additional arguments
shift
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-tests)
            SKIP_TESTS=true
            shift
            ;;
        --skip-build)
            SKIP_BUILD=true
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
    print_error "Invalid version format. Expected: vX.Y.Z (e.g., v0.2.0)"
    exit 1
fi

print_info "Preparing release $VERSION"

# Check if release already exists
if [ -d "releases/$VERSION" ]; then
    print_error "Release $VERSION already exists"
    exit 1
fi

# Run tests unless skipped
if [ "$SKIP_TESTS" = "false" ]; then
    print_info "Running tests..."
    ./scripts/test.sh
    if [ $? -ne 0 ]; then
        print_error "Tests failed. Release aborted."
        exit 1
    fi
    print_info "✓ Tests passed"
else
    print_warn "Skipping tests"
fi

# Build binaries unless skipped
if [ "$SKIP_BUILD" = "false" ]; then
    print_info "Building binaries for all platforms..."
    VERSION=$VERSION ./scripts/build.sh --all
    if [ $? -ne 0 ]; then
        print_error "Build failed. Release aborted."
        exit 1
    fi
    print_info "✓ Build completed"
else
    print_warn "Skipping build"
fi

# Create release directory
RELEASE_DIR="releases/$VERSION"
mkdir -p "$RELEASE_DIR"

print_info "Creating release archives..."

# Create archives for each platform
for platform_dir in binaries/*/; do
    if [ -d "$platform_dir" ]; then
        platform=$(basename "$platform_dir")
        print_info "Packaging $platform..."
        
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
print_info "Updated release.txt to $VERSION"

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
docker run -p 8080:8080 ghcr.io/apimgr/search:$VERSION
\`\`\`

## Full Changelog

See [CHANGELOG.md](../../CHANGELOG.md) for detailed changes.
EOF

print_info "Created release notes template at $RELEASE_DIR/RELEASE_NOTES.md"

# Summary
echo ""
echo -e "${BLUE}=== Release Summary ===${NC}"
echo -e "${GREEN}Version:${NC} $VERSION"
echo -e "${GREEN}Release directory:${NC} $RELEASE_DIR"
echo -e "${GREEN}Archives created:${NC}"
ls -lh "$RELEASE_DIR"/*.{tar.gz,zip} 2>/dev/null || true
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Edit $RELEASE_DIR/RELEASE_NOTES.md with actual changes"
echo "2. Update CHANGELOG.md"
echo "3. Commit changes: git add . && git commit -m 'Release $VERSION'"
echo "4. Create tag: git tag -a $VERSION -m 'Release $VERSION'"
echo "5. Push changes: git push && git push --tags"
echo "6. Create GitHub release with archives from $RELEASE_DIR"
echo ""
echo -e "${GREEN}=== Release $VERSION Prepared ===${NC}"
