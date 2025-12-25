# Search

A privacy-respecting metasearch engine that aggregates results from multiple search engines while protecting your privacy.

## Features

- **Privacy-First**: No tracking, no logging of search queries, no personal data collection
- **Multiple Engines**: Aggregates results from DuckDuckGo, Google, Bing, Brave, and more
- **Self-Hosted**: Run your own search engine instance
- **Dark Theme**: Beautiful Dracula-inspired dark theme
- **Mobile-Friendly**: Responsive design works on all devices
- **API Access**: Full REST API and GraphQL support
- **Bang Commands**: Quick shortcuts to search other sites (e.g., `!g` for Google, `!w` for Wikipedia)
- **Image Proxy**: Proxies images to protect your privacy
- **Tor Support**: Built-in Tor hidden service support

## Quick Start

=== "Docker"

    ```bash
    docker run -d \
      --name search \
      -p 8080:80 \
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

Then open [http://localhost:8080](http://localhost:8080) in your browser.

## Documentation

- [Installation](installation.md) - Detailed installation instructions
- [Configuration](configuration.md) - Configuration options and settings
- [API](api.md) - REST and GraphQL API documentation
- [Admin Panel](admin.md) - Administration guide
- [CLI](cli.md) - Command-line interface reference

## Requirements

- **Operating System**: Linux, macOS, Windows, or FreeBSD
- **Architecture**: AMD64 or ARM64
- **Memory**: 256MB minimum, 512MB recommended
- **Disk**: 100MB for the application

## License

Search is released under the [MIT License](https://github.com/apimgr/search/blob/main/LICENSE.md).
