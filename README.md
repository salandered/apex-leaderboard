# Apex

![alt text](logo.png)

Apex is a backend web service for leaderboards, built as an MVP to prove out two things:

- **Architecturally:** every score is event-sourced. An event ledger is the source of truth, and
  leaderboards are derived views.
- **Technically:** Redis is the only datastore. Beyond plain key-value use, it acts as a persistent
  document database and as the main store for the event-sourced parts: topics (streams), views, and
  consumer data.

More details in [docs/architecture.md](docs/architecture.md)

## 🚀 Quick Start

You will only need [Docker](https://docs.docker.com/get-docker/). Run the app:

```bash
docker compose up --build
```

Make requests:

```bash
# Create a player profile; returns the generated id
curl -X POST http://localhost:8090/api/v1/players \
  -H "Content-Type: application/json" \
  -d '{"player_name":"alice"}'
# {"player_id":"7dcbeb46-e1e1-492d-a32a-c593b13428de"}

# Give that player a score on the default "main" board (change UUID)
curl -X PUT http://localhost:8090/api/v1/boards/main/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de \
  -H "Content-Type: application/json" \
  -d '{"player_score":42.5}'

# See the leaderboard
curl "http://localhost:8090/api/v1/boards/main/scores"

# Read all score events from the beginning
curl "http://localhost:8090/api/v1/events?after=0-0"
```

## API Spec

[`api.yaml`](apispec/api.yaml) - OpenAPI specification

## 🛠️ Development

[Go](https://go.dev/doc/install) 1.26+ is used in addition to Docker.

### Running the Server

**Everything in Docker (app + Redis)**

```bash
docker compose up -d --build   # app on :8090, Redis on 127.0.0.1:6379 (data persisted in a volume)
docker compose logs -f app     # follow the app logs
docker compose down            # stop the stack (add -v to wipe Redis data)
```

**Locally with Go (Redis in Docker)**

```bash
docker compose up -d redis # or: docker run -p 6379:6379 redis:8.8.0-alpine
go run .
```

The server listens on port `:8090` and connects to Redis via `REDIS_URL`
(default `redis://localhost:6379/0`).

### Configuration

All envs are optional:

| Variable     | Values                        | Default                    | Description                                                                                                              |
| ------------ | ----------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `REDIS_URL`  | Redis connection URL          | `redis://localhost:6379/0` | Storage url.                                                                                                             |
| `LOG_LEVEL`  | `debug` `info` `warn` `error` | `info`                     | Minimum log level being printed.                                                                                         |
| `LOG_FORMAT` | `text` `json`                 | `text`                     | `text` is a human readable format (colorized if using stdout), `json` is for machines.                                   |
| `LOG_FILE`   | file path                     | *(empty → stdout)*         | If set, logs go to this file only.                                                                                       |
| `LOG_TIME`   | `short` `nano`                | `short`                    | `text` timestamp precision; `nano` adds fractional seconds. Does not affect `LOG_FORMAT = json` (always full precision). |

For example,

```bash
LOG_LEVEL=debug LOG_FORMAT=text LOG_FILE=./apex.log go run .
```

will be logging messages like `05:23:40 INFO starting server addr=:8090` into file.

### API Examples

With the server running on `:8090`:

```bash
# Create a player profile; returns the generated id.
curl -X POST http://localhost:8090/api/v1/players \
  -H "Content-Type: application/json" \
  -d '{"player_name":"alice"}'
# {"player_id":"7dcbeb46-e1e1-492d-a32a-c593b13428de"}

# Set a score on a main board
curl -X PUT http://localhost:8090/api/v1/boards/main/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de \
  -H "Content-Type: application/json" \
  -d '{"player_score":36}'

# Retry-safe increment: send an Idempotency-Key. Try to curl this several times with and without the header.
curl -X POST http://localhost:8090/api/v1/boards/main/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de/increment \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: r1" \
  -d '{"amount":5}'

# See all the score events
curl http://localhost:8090/api/v1/boards/main/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de/history

# Read the global score event feed, oldest first
curl "http://localhost:8090/api/v1/events?after=0-0&limit=50"

# Verify a leaderboard's projection against its ledger
curl http://localhost:8090/api/v1/admin/boards/main/projection/verify
# {"mismatches":[]}

# Create a new closed board
curl -X PUT http://localhost:8090/api/v1/boards/summer-contest \
  -H "Content-Type: application/json" \
  -d '{"board_name":"Summer Contest","status":"closed"}'

# Try to set a score on a new closed board
curl -X PUT http://localhost:8090/api/v1/boards/summer-contest/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de \
  -H "Content-Type: application/json" \
  -d '{"player_score":12}'

# List boards
curl http://localhost:8090/api/v1/boards
```

### API Walk

Exercises every endpoint against a running server:

```bash
go run ./apiscripts/apiwalk -base-url http://localhost:8090 -board demo-cup
```

### Run Tests

```bash
go test ./...                    # unit tests
go test -tags=integration ./...  # integration tests with db (uses Docker)
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

Run `tidy` to sync `go.mod` and `go.sum` with the actual imports.

```bash
go mod tidy
```

### Developer Docs

- See docs/ folder.

## Misc

No connection with the Apex Legends game whatsoever.
