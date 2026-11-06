# Installation

Search can be installed in several ways depending on your environment and preferences.

Commands that install binaries, change system services, or manage Docker on a
host are for the operator to run on the deployment host; do not have an AI
agent execute them.

## Docker (Recommended)

The easiest way to run Search is using Docker:

```bash
docker run -d \
  --name search-app \
  -p 172.17.0.1:64580:80 \
  -v search_data:/data \
  -v search_config:/config \
  ghcr.io/apimgr/search:latest
```

### Docker Compose

Use the repository's production Compose file from a deployment directory outside
the source checkout. Its relative `volumes/` mounts then keep runtime data out of
the project tree.

```bash
git clone https://github.com/apimgr/search.git search-source
DEPLOY_DIR="$HOME/search-deployment"
mkdir -p "$DEPLOY_DIR"
cp search-source/docker/docker-compose.yml "$DEPLOY_DIR/docker-compose.yml"
cd "$DEPLOY_DIR"
docker compose up -d
```

## Binary Installation

### Download

Download the latest release for your platform from the [releases page](https://github.com/apimgr/search/releases).

=== "Linux (AMD64)"

    ```bash
    curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
    chmod +x search-linux-amd64
    sudo mv search-linux-amd64 /usr/local/bin/search
    ```

=== "Linux (ARM64)"

    ```bash
    curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-linux-arm64
    chmod +x search-linux-arm64
    sudo mv search-linux-arm64 /usr/local/bin/search
    ```

=== "macOS (AMD64)"

    ```bash
    curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-darwin-amd64
    chmod +x search-darwin-amd64
    xattr -d com.apple.quarantine search-darwin-amd64 2>/dev/null || true
    sudo mv search-darwin-amd64 /usr/local/bin/search
    ```

=== "macOS (ARM64)"

    ```bash
    curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-darwin-arm64
    chmod +x search-darwin-arm64
    xattr -d com.apple.quarantine search-darwin-arm64 2>/dev/null || true
    sudo mv search-darwin-arm64 /usr/local/bin/search
    ```

=== "Windows (AMD64)"

    ```powershell
    Invoke-WebRequest -Uri https://github.com/apimgr/search/releases/latest/download/search-windows-amd64.exe -OutFile search.exe
    .\search.exe
    ```

=== "Windows (ARM64)"

    ```powershell
    Invoke-WebRequest -Uri https://github.com/apimgr/search/releases/latest/download/search-windows-arm64.exe -OutFile search.exe
    .\search.exe
    ```

=== "FreeBSD (AMD64)"

    ```sh
    fetch https://github.com/apimgr/search/releases/latest/download/search-freebsd-amd64
    chmod +x search-freebsd-amd64
    sudo install -m 0755 search-freebsd-amd64 /usr/local/bin/search
    ```

=== "FreeBSD (ARM64)"

    ```sh
    fetch https://github.com/apimgr/search/releases/latest/download/search-freebsd-arm64
    chmod +x search-freebsd-arm64
    sudo install -m 0755 search-freebsd-arm64 /usr/local/bin/search
    ```

The same eight operating system and architecture combinations are published
for `search-cli`. Use the [GitHub releases page](https://github.com/apimgr/search/releases)
to select the matching `search-cli-{os}-{arch}` asset; Windows assets end in
`.exe`.

### Running as a Service

#### Systemd (Linux)

Install and start the managed systemd service:

```bash
sudo search --service --install
```

## First Run

For the Docker command above, access the web interface at `http://172.17.0.1:64580`. A binary installation prints its selected URL when it starts.

On first run, Search auto-generates `server.yml` with defaults including a random operator token. The startup banner shows the server URL and the token location. No setup wizard or admin account is required.

## Upgrading

### Docker

```bash
docker pull ghcr.io/apimgr/search:latest
docker stop search-app
docker rm search-app
docker run -d \
  --name search-app \
  -p 172.17.0.1:64580:80 \
  -v search_data:/data \
  -v search_config:/config \
  ghcr.io/apimgr/search:latest
```

### Binary

```bash
search --update yes
```

Or manually:

```bash
# Stop the service
sudo systemctl stop search

# Download new version
curl -q -LSsfO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
chmod +x search-linux-amd64
sudo mv search-linux-amd64 /usr/local/bin/search

# Start the service
sudo systemctl start search
```
