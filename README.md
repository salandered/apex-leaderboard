# Apex

Apex is the backend web service implementing the Leaderboard functionality. A lightweight HTTP API for storing and retrieving player scores using Redis (inspired by https://redis.io/solutions/leaderboards/).

## 🚀 Quick Start

### Prerequisites

Install 1.26+ [Go](https://go.dev/doc/install)

### Run the Server

```bash
go run main.go
```

The server listens on port `:8090`.

###

TODO: add curls with basic functionality

## 📡 API

See OpenAPI specification [`api.yaml`](api.yaml)

## 🛠️ Development

### Run Tests

```bash
go test ./...
```

### Compile

**Windows** (explicitly add the `.exe` extension):

```bash
go build -o apex.exe .
.\apex.exe
```

**Mac/Linux**:

```bash
go build -o apex .
./apex
```

### Dependencies

Run `go mod tidy` to sync `go.mod` and `go.sum` with the actual imports.

```bash
go mod tidy
```
