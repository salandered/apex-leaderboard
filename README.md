# Apex

Apex is the backend web service implementing the Leaderboard functionality. A lightweight HTTP API for storing and retrieving player scores using Redis (inspired by https://redis.io/solutions/leaderboards/).

## 🚀 Quick Start

### Prerequisites

- 1.26+ [Go](https://go.dev/doc/install)
- A running Redis for the server (e.g. `docker run -p 6379:6379 redis:8.8.0-alpine`)
- [Docker](https://docs.docker.com/get-docker/) — only for the integration tests

### Run the Server

```bash
go run main.go
```

The server listens on port `:8090` and connects to Redis via `REDIS_URL`
(default `redis://localhost:6379/0`). Point it elsewhere with the env var:

```bash
REDIS_URL=redis://:password@host:6379/0 go run main.go
```

###

TODO: add curls with basic functionality

## 📚 Documentation

- [`api.yaml`](api.yaml) - OpenAPI specification
- [docs/tests.md](docs/tests.md) - testing approach
- [docs/redis.md](docs/redis.md) - go-redis references

## 🛠️ Development

### Run Tests

```bash
go test ./...                    # unit tests — fast, no Docker
go test -tags=integration ./...  # + storage integration tests (needs Docker)
```

See [docs/tests.md](docs/tests.md) for details.

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
