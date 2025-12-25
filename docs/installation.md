# Installation

Search can be installed in several ways depending on your environment and preferences.

## Docker (Recommended)

The easiest way to run Search is using Docker:

```bash
docker run -d \
  --name search \
  -p 8080:80 \
  -v search_data:/data \
  -v search_config:/config \
  ghcr.io/apimgr/search:latest
```

### Docker Compose

For a more complete setup with persistent configuration:

```yaml
version: '3.8'

services:
  search:
    image: ghcr.io/apimgr/search:latest
    container_name: search
    ports:
      - "8080:80"
    volumes:
      - ./config:/config
      - ./data:/data
    environment:
      - MODE=production
      - TZ=America/New_York
    restart: unless-stopped
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

Create a systemd service file:

```bash
sudo search --service install
sudo systemctl enable search
sudo systemctl start search
```

Or manually create `/etc/systemd/system/search.service`:

```ini
[Unit]
Description=Search - Privacy-Respecting Metasearch Engine
After=network.target

[Service]
Type=simple
User=search
Group=search
ExecStart=/usr/local/bin/search
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable search
sudo systemctl start search
```

## First Run

After installation, access the web interface at `http://localhost:8080` (or your configured port).

On first run, you'll be prompted to complete the setup wizard to create an admin account.

## Upgrading

### Docker

```bash
docker pull ghcr.io/apimgr/search:latest
docker stop search
docker rm search
docker run -d \
  --name search \
  -p 8080:80 \
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
