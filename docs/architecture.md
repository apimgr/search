# Architecture

This document describes the high-level architecture of Search.

## Overview

Search is a single-binary application written in Go. It aggregates search results from multiple engines while protecting user privacy.

```
┌─────────────────────────────────────────────────────────┐
│                      Load Balancer                       │
└─────────────────────────────────────────────────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
    ┌──────────┐      ┌──────────┐      ┌──────────┐
    │  Search  │      │  Search  │      │  Search  │
    │  Node 1  │      │  Node 2  │      │  Node 3  │
    └──────────┘      └──────────┘      └──────────┘
          │                 │                 │
          └─────────────────┼─────────────────┘
                            │
                    ┌───────┴───────┐
                    │   Database    │
                    │   (SQLite/    │
                    │   PostgreSQL) │
                    └───────────────┘
```

## Components

### HTTP Server

The HTTP server handles all incoming requests:

- **Routes**: Web UI, REST API, GraphQL, Admin panel
- **Middleware**: Logging, CSRF, rate limiting, compression
- **Template Rendering**: Go templates with Dracula theme

### Search Aggregator

The search aggregator coordinates queries across multiple engines:

```
        ┌──────────────────────────────────────┐
        │           Search Aggregator           │
        └──────────────────────────────────────┘
                          │
     ┌────────────────────┼────────────────────┐
     ▼                    ▼                    ▼
┌─────────┐        ┌─────────┐          ┌─────────┐
│DuckDuck │        │  Google │          │  Bing   │
│   Go    │        │         │          │         │
└─────────┘        └─────────┘          └─────────┘
```

Features:
- Concurrent queries to multiple engines
- Result deduplication
- Result ranking and scoring
- Response caching

### Engine Registry

Engines are registered in a central registry:

```go
type Registry struct {
    engines map[string]Engine
}

type Engine interface {
    Name() string
    Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error)
    Categories() []Category
    IsEnabled() bool
}
```

### Configuration System

Configuration follows a priority chain:

```
CLI Flags > Environment Variables > Config File > Defaults
```

Configuration can be reloaded at runtime without restarting the server.

### Database Layer

SQLite is used for local storage:

- Session management
- API tokens
- User accounts
- Audit logs
- Settings

For cluster deployments, PostgreSQL or MySQL can be used.

### Scheduler

Built-in task scheduler for periodic operations:

- Automatic backups (daily)
- SSL certificate renewal (daily check)
- GeoIP database updates (weekly)
- Session cleanup (hourly)

### Service Manager

Platform-specific service management:

- **Linux**: systemd, runit
- **macOS**: launchd
- **Windows**: Windows Service Manager
- **BSD**: rc.d

## Request Flow

### Search Request

```
1. User submits query
2. Rate limiter checks
3. Query parsed (bang detection, operators)
4. Cache check
5. If not cached:
   a. Query sent to enabled engines (concurrent)
   b. Results aggregated and deduplicated
   c. Results ranked and scored
   d. Results cached
6. Response rendered (HTML/JSON/GraphQL)
7. Access logged (without query)
```

### Admin Request

```
1. Request received
2. Session cookie validated
3. CSRF token validated
4. Authorization check
5. Action performed
6. Audit log entry created
7. Response rendered
```

## Security Architecture

### Authentication

```
┌───────────────────────────────────────┐
│              Auth Manager             │
├───────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐    │
│  │   Session   │  │   API       │    │
│  │   Auth      │  │   Token     │    │
│  └─────────────┘  └─────────────┘    │
│  ┌─────────────┐  ┌─────────────┐    │
│  │    TOTP     │  │   OIDC      │    │
│  │    2FA      │  │   (opt)     │    │
│  └─────────────┘  └─────────────┘    │
└───────────────────────────────────────┘
```

### Data Flow

- Search queries are **never** logged
- IP addresses are hashed in access logs
- Images are proxied to prevent tracking
- All external requests go through aggregator

## Cluster Mode

In cluster mode, multiple nodes share state:

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│  Node 1  │◄───►│  Node 2  │◄───►│  Node 3  │
│(Primary) │     │(Replica) │     │(Replica) │
└──────────┘     └──────────┘     └──────────┘
      │                │                │
      └────────────────┼────────────────┘
                       ▼
              ┌──────────────┐
              │  PostgreSQL  │
              └──────────────┘
```

Features:
- Distributed task locking
- Shared session storage
- Leader election for scheduled tasks

## Extensibility

### Adding Search Engines

Engines implement the `Engine` interface and register with the registry.

### Adding Widgets

Dashboard widgets implement the `Widget` interface:

```go
type Widget interface {
    Name() string
    Render(ctx context.Context) (template.HTML, error)
}
```

### Adding Instant Answers

Instant answer providers implement:

```go
type InstantProvider interface {
    Match(query string) bool
    Answer(ctx context.Context, query string) (*InstantAnswer, error)
}
```
