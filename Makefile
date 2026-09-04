.PHONY: build run test clean install

# Version and commit are baked into the binary so a running copy can be matched
# to the source it came from. The nix build passes the same values via package.nix.
VERSION := $(shell cat VERSION)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.appVersion=$(VERSION) -X main.appCommit=$(COMMIT)

# Default target
all: build

# Build the application
build:
	go build -ldflags "$(LDFLAGS)" -o zfs-backup .

# Run the application
run:
	go run -ldflags "$(LDFLAGS)" .

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f zfs-backup

# Install to GOPATH/bin
install:
	go install .

# Build for all platforms
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o zfs-backup-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o zfs-backup-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o zfs-backup-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o zfs-backup-windows-amd64.exe .
