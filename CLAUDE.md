# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ReelTube CLI is a command-line interface for interacting with the ReelTube API (api.reel.tube). It provides commands for authentication verification and media file uploads using multipart upload with concurrent workers.

## Development Commands

### Building
```bash
go build -o reeltube ./cmd/reeltube
```

### Building with GoReleaser (for releases)
```bash
goreleaser release --snapshot --clean
```

### Running the CLI
```bash
# Verify authentication
./reeltube whoami -k YOUR_API_KEY

# Upload a file
./reeltube upload -f path/to/file.mp4

# Use environment variable for API key
export REELTUBE_API_KEY=your_api_key
./reeltube whoami

# Enable debug mode
./reeltube whoami -k YOUR_API_KEY -d

# Check version
./reeltube version
```

### Dependency Management
```bash
go mod tidy
go mod download
```

## Architecture

### Code Structure
- `main.go` - CLI entry point with Cobra command definitions (whoami, upload, version)
- `client.go` - HTTP client for ReelTube API with JSON request/response handling
- `upload.go` - Multipart upload implementation with concurrent workers

### Key Components

**API Client (`client.go`)**
- Generic HTTP methods (GET, POST, PUT, DELETE) with Bearer token authentication
- Debug mode for request/response logging
- Handles JSON marshalling/unmarshalling and error responses
- Domain-specific methods: `Me()`, `CreateMediaUpload()`, `CompleteMultipartUpload()`

**Upload System (`upload.go`)**
- Validates file extensions (jpg, jpeg, png, gif, mp4, mov, avi, mkv) and MIME types
- Uses worker pool pattern for concurrent part uploads (defaults to system CPU count)
- Progress bar with ETA using cheggaaa/pb/v3
- Three-phase process:
  1. Request presigned URLs from API
  2. Upload file parts concurrently to presigned URLs
  3. Complete multipart upload via API

**CLI Structure (`main.go`)**
- Built with spf13/cobra for command handling
- Global flags: `--api-key`, `--base-url`, `--debug`
- API key can be provided via flag or `REELTUBE_API_KEY` environment variable
- Version info injected via ldflags during build (version, commit, date)
- Loads `.env` file automatically with godotenv

### Configuration
Environment variables (optional .env file):
- `REELTUBE_API_KEY` - API authentication key
- `REELTUBE_API_HOST` - API base URL (defaults to https://api.reel.tube)

### Cross-Platform Builds
GoReleaser is configured to build for:
- Linux, Windows, Darwin (macOS)
- Archives generated with version-tagged names
- Uses goreleaser's default ldflags for version injection

## Important Implementation Details

### Upload Concurrency
The upload system automatically determines concurrency based on `runtime.NumCPU()`. Each worker:
- Opens its own file descriptor to avoid seeking conflicts
- Reads the assigned part from the file
- Uploads to S3 via presigned URL
- Collects ETag from response for completion

### Error Handling
- File validation happens before upload initialization (extension, MIME type, existence)
- Worker errors are collected via error channel and checked after all workers complete
- Failed uploads return immediately without completing multipart upload

### API Response Structure
Standard API response wrapper used throughout:
```go
type APIResponse struct {
    Data   interface{}
    Error  string
    Status int
}
```

### Go Module Issue
Note: `go.mod:12` has a warning that `github.com/cheggaaa/pb/v3` should be direct. This is currently marked as indirect but is used directly in `upload.go`.
