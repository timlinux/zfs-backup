.PHONY: build run test clean install

# Default target
all: build

# Build the application
build:
	go build -o zfs-backup .

# Run the application
run:
	go run .

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
	GOOS=linux GOARCH=amd64 go build -o zfs-backup-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -o zfs-backup-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o zfs-backup-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build -o zfs-backup-windows-amd64.exe .
