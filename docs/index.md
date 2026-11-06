# Search

A privacy-respecting metasearch engine that aggregates results from multiple search engines while protecting your privacy.

## Features

- **Privacy-First**: No third-party analytics by default, consent-aware preferences, no hosted user profiling
- **Multiple Engines**: Aggregates results from DuckDuckGo, Google, Bing, Brave, and more
- **Self-Hosted**: Run your own search engine instance
- **Themes**: Dark, light, and automatic themes with configurable typography and result density
- **Mobile-Friendly**: Responsive design works on all devices
- **API Access**: Full REST API and GraphQL support
- **Bang Commands**: Quick shortcuts to search other sites (e.g., `!g` for Google, `!w` for Wikipedia)
- **Search Syntax**: Boolean operators, exact phrases, and full-page direct answers
- **Image Proxy**: Proxies images to protect your privacy
- **Tor Support**: Built-in Tor hidden service support
- **CLI Companion**: `search-cli` provides the required operator and search client

## Quick Start

=== "Docker"

    ```bash
    docker run -d \
      --name search-app \
      -p 172.17.0.1:64580:80 \
      -v search_config:/config \
      -v search_data:/data \
      ghcr.io/apimgr/search:latest
    ```

=== "Binary"

    ```bash
    # Download the latest release
    curl -LO https://github.com/apimgr/search/releases/latest/download/search-linux-amd64
    chmod +x search-linux-amd64
    ./search-linux-amd64
    ```

For Docker, open [http://172.17.0.1:64580](http://172.17.0.1:64580). The binary prints its selected URL when it starts.

## Documentation

- [Installation](installation.md) - Detailed installation instructions
- [Configuration](configuration.md) - Configuration options and settings
- [API](api.md) - REST and GraphQL API documentation
- [CLI](cli.md) - Command-line interface reference
- [Search](search.md) - Search operators and direct-answer syntax

## Requirements

- **Operating System**: Linux, macOS, Windows, or FreeBSD
- **Architecture**: AMD64 or ARM64
- **Memory**: 256MB minimum, 512MB recommended
- **Disk**: 100MB for the application

## License

Search is released under the [MIT License](https://github.com/apimgr/search/blob/main/LICENSE.md).
