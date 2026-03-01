# Building

This guide covers building Kartoza ZFS Backup Tool from source.

## Prerequisites

### Using Nix (Recommended)

With Nix installed, all dependencies are handled automatically:

```bash
# Enter development shell
nix develop

# All tools are now available
go version
```

### Manual Setup

Without Nix, ensure you have:

- Go 1.21 or later
- Git

## Development Shell

The Nix flake provides a complete development environment:

```bash
# Enter the shell
nix develop

# Available commands:
go build     # Build the binary
go test      # Run tests
go run .     # Run directly
```

## Building

### Using Make

```bash
# Build for current platform
make build

# Run the built binary
./zfs-backup

# Clean build artifacts
make clean

# Build for all platforms
make build-all
```

### Using Go Directly

```bash
# Simple build
go build -o zfs-backup .

# With version info
go build -ldflags "-X main.version=1.0.0" -o zfs-backup .

# Race detector (for testing)
go build -race -o zfs-backup .
```

### Using Nix

```bash
# Build the package
nix build

# Run the built binary
./result/bin/zfs-backup

# Build and run in one step
nix run
```

## Cross-Compilation

### Linux (AMD64)

```bash
GOOS=linux GOARCH=amd64 go build -o zfs-backup-linux-amd64 .
```

### Linux (ARM64)

```bash
GOOS=linux GOARCH=arm64 go build -o zfs-backup-linux-arm64 .
```

### macOS (AMD64)

```bash
GOOS=darwin GOARCH=amd64 go build -o zfs-backup-darwin-amd64 .
```

### macOS (ARM64 / Apple Silicon)

```bash
GOOS=darwin GOARCH=arm64 go build -o zfs-backup-darwin-arm64 .
```

## Dependencies

### Go Modules

Dependencies are managed via Go modules:

```bash
# Download dependencies
go mod download

# Update dependencies
go get -u ./...

# Tidy up
go mod tidy
```

### Key Dependencies

| Package | Purpose |
|---------|---------|
| github.com/charmbracelet/bubbletea | TUI framework |
| github.com/charmbracelet/bubbles | TUI components |
| github.com/charmbracelet/lipgloss | Terminal styling |

## Testing

```bash
# Run all tests
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Linting

```bash
# Using golangci-lint (if installed)
golangci-lint run

# Using go vet
go vet ./...

# Format code
go fmt ./...
```

## Documentation

### Building MkDocs

```bash
# Install dependencies
pip install mkdocs-material mkdocs-minify-plugin

# Serve locally
mkdocs serve

# Build static site
mkdocs build
```

### Viewing Locally

```bash
mkdocs serve
# Open http://localhost:8000
```

## Release Process

1. Update version in `main.go`
2. Update CHANGELOG (if exists)
3. Commit changes
4. Tag the release:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
5. GitHub Actions will build and publish

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
