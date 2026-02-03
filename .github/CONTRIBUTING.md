# Contributing to Search

Thank you for your interest in contributing to Search! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Process](#development-process)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

This project adheres to the [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR-USERNAME/search.git
   cd search
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/apimgr/search.git
   ```
4. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Process

### Prerequisites

- Docker (recommended for consistent build environment)
- Go 1.21+ (if building locally)
- Git

### Building

Using Docker (recommended):
```bash
docker run --rm -v $(pwd):/app -w /app golang:latest go build -o binaries/search src/main.go
```

Local build:
```bash
go build -o binaries/search src/main.go
```

### Running Tests

```bash
# Using Docker
docker run --rm -v $(pwd):/app -w /app golang:latest go test ./...

# Local
go test ./...
```

### Running the Application

```bash
./binaries/search
```

Access at http://localhost:64580

## Pull Request Process

1. **Update your branch** with the latest upstream changes:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Make your changes** following the [Coding Standards](#coding-standards)

3. **Test your changes** thoroughly:
   - Run all tests: `go test ./...`
   - Test manually in browser
   - Test different search categories (web, images, videos, etc.)

4. **Commit your changes** with clear, descriptive messages:
   ```bash
   git commit -m "Add feature: description of feature"
   ```

5. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

6. **Create a Pull Request** on GitHub:
   - Provide a clear title and description
   - Reference any related issues
   - Include screenshots for UI changes
   - List any breaking changes

7. **Address review feedback** promptly

### Pull Request Requirements

- [ ] Code follows project style guidelines
- [ ] Tests pass successfully
- [ ] Documentation updated (if applicable)
- [ ] CHANGELOG.md updated
- [ ] No merge conflicts
- [ ] Commits are clean and well-organized

## Coding Standards

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` to format code
- Use meaningful variable and function names
- Keep functions small and focused
- Add comments for exported functions and types

### File Organization

```
src/
├── main.go           # Entry point
├── config/           # Configuration management
├── models/           # Data models
├── search/           # Search logic
│   └── engines/      # Search engine implementations
└── server/           # HTTP server and handlers
```

### Naming Conventions

- **Packages**: lowercase, single word (e.g., `search`, `config`)
- **Files**: lowercase with underscores (e.g., `engine_registry.go`)
- **Types**: PascalCase (e.g., `SearchEngine`, `QueryParams`)
- **Functions/Methods**: PascalCase for exported, camelCase for private
- **Constants**: PascalCase or SCREAMING_SNAKE_CASE for environment variables

### Error Handling

- Always check and handle errors
- Use descriptive error messages
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Log errors appropriately

### Adding Search Engines

To add a new search engine:

1. Create new file in `src/search/engines/`
2. Implement the `SearchEngine` interface
3. Register in `src/search/engines/registry.go`
4. Add configuration options
5. Add tests
6. Update documentation

Example:
```go
type NewEngine struct {
    BaseURL string
}

func (e *NewEngine) Search(ctx context.Context, query models.QueryParams) ([]models.Result, error) {
    // Implementation
}

func (e *NewEngine) GetInfo() models.EngineInfo {
    return models.EngineInfo{
        Name:     "newengine",
        Category: models.CategoryWeb,
        // ...
    }
}
```

## Testing

### Writing Tests

- Write unit tests for new functions
- Test edge cases and error conditions
- Use table-driven tests when appropriate
- Mock external dependencies

Example test structure:
```go
func TestSearchEngine(t *testing.T) {
    tests := []struct {
        name    string
        query   string
        want    int
        wantErr bool
    }{
        {"basic search", "golang", 10, false},
        {"empty query", "", 0, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Test Coverage

Aim for reasonable test coverage, especially for:
- Search engine implementations
- Result aggregation and deduplication
- Configuration parsing
- Security-critical code (input sanitization, etc.)

## Documentation

### Code Documentation

- Document all exported types, functions, and methods
- Use godoc format for comments
- Include examples where helpful

### User Documentation

Update relevant documentation when making changes:
- **README.md**: User-facing features and usage
- **SPEC.md**: Technical specifications and architecture
- **CHANGELOG.md**: All notable changes

## Issue Reporting

### Bug Reports

When reporting bugs, include:
- Search version and Go version
- Operating system
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or screenshots

### Feature Requests

For feature requests, describe:
- The problem you're trying to solve
- Proposed solution
- Alternative solutions considered
- Any relevant examples from other projects

## Community

- Be respectful and inclusive
- Provide constructive feedback
- Help others when you can
- Share your knowledge and experience

## Questions?

If you have questions:
- Check existing issues and documentation
- Open a new issue with the "question" label
- Use the contact form on the website (if configured)

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see LICENSE.md).

Thank you for contributing to Search! 🔍
