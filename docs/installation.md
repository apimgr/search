# Installation

Search can be installed in several ways depending on your environment and preferences.

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
    curl -LO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
    chmod +x search-linux-amd64
    sudo mv search-linux-amd64 /usr/local/bin/search
    ```

=== "Linux (ARM64)"

    ```bash
    curl -LO https://github.com/apimgr/search/releases/latest/download/search-linux-arm64
    chmod +x search-linux-arm64
    sudo mv search-linux-arm64 /usr/local/bin/search
    ```

=== "macOS (AMD64)"

    ```bash
    curl -LO https://github.com/apimgr/search/releases/latest/download/search-darwin-amd64
    chmod +x search-darwin-amd64
    sudo mv search-darwin-amd64 /usr/local/bin/search
    ```

=== "macOS (ARM64)"

    ```bash
    curl -LO https://github.com/apimgr/search/releases/latest/download/search-darwin-arm64
    chmod +x search-darwin-arm64
    sudo mv search-darwin-arm64 /usr/local/bin/search
    ```

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
curl -LO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
chmod +x search-linux-amd64
sudo mv search-linux-amd64 /usr/local/bin/search

# Start the service
sudo systemctl start search
```
