# Apex

- [🚀 Quick Start](#-quick-start)
- [API Spec](#api-spec)
- [🛠️ Development](#️-development)
	- [Running the Server](#running-the-server)
	- [Web UI](#web-ui)
	- [Configuration](#configuration)
	- [API Walk](#api-walk)
	- [Run Tests](#run-tests)
	- [Compile](#compile)
- [Misc](#misc)

---

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
# Create a player profile and keep the generated id for the calls below.
PLAYER_ID=$(curl -s -X POST http://localhost:8090/api/v1/players \
  -H "Content-Type: application/json" \
  -d '{"player_name":"alice"}' \
  | sed -E 's/.*"player_id":"([^"]+)".*/\1/')

echo "$PLAYER_ID"
# 7dcbeb46-e1e1-492d-a32a-c593b13428de

# Create a new board
curl -X PUT http://localhost:8090/api/v1/boards/main \
  -H "Content-Type: application/json" \
  -d '{"board_name":"Main"}'

# Set a score on a new board
curl -X PUT "http://localhost:8090/api/v1/boards/main/scores/$PLAYER_ID" \
  -H "Content-Type: application/json" \
  -d '{"player_score":36}'

# Retry-safe increment: send an Idempotency-Key. Try to curl this several times with and without the header.
curl -X POST "http://localhost:8090/api/v1/boards/main/scores/$PLAYER_ID/increment" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: r1" \
  -d '{"amount":5}'

# See the leaderboard
curl "http://localhost:8090/api/v1/boards/main/scores"

# See all the score events
curl "http://localhost:8090/api/v1/boards/main/scores/$PLAYER_ID/history"

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
curl -X PUT "http://localhost:8090/api/v1/boards/summer-contest/scores/$PLAYER_ID" \
  -H "Content-Type: application/json" \
  -d '{"player_score":12}'

# List boards
curl http://localhost:8090/api/v1/boards
```

## API Spec

[`api.yaml`](apispec/api.yaml) - OpenAPI specification

## 🛠️ Development

ℹ️ More developer docs: see `docs/` folder.

[Go](https://go.dev/doc/install) 1.26+ is used in addition to Docker.

### Running the Server

**Everything in Docker (app + Redis)**

```bash
docker compose up -d --build   # data persisted in a volume
docker compose logs -f app     # follow the app logs
docker compose down            # stop the stack
```

The API is on `http://localhost:8090`, Redis on `127.0.0.1:6379`. Both are published on loopback
only.

**Locally with Go (Redis in Docker)**

```bash
docker compose up -d redis # or: docker run -p 6379:6379 redis:8.8.0-alpine
go run .
```

### Web UI

Optional, not needed for anything above and does not affect anything.
A static page for reading and changing leaderboards, served by Caddy.

```bash
docker compose -f docker-compose.yml -f docker-compose.web.yml up -d
```

Then open `http://localhost:8089`.

Development notes and the frontend data flow are in [web/README.md](web/README.md).

ℹ️ Currently works but WIP.

### Configuration

All envs are optional, but a set and invalid one will abort the start up.
`UI_PORT` is the exception: it is read by `docker compose`, not by the app.

| Variable           | Values                                                           | Default                                                        | Description                                                                                                       |
| ------------------ | ---------------------------------------------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `PORT`             | `1`..`65535`                                                     | `8090`                                                         | HTTP listen port.                                                                                                 |
| `UI_PORT`          | `1`..`65535`                                                     | `8089`                                                         | Compose only: host port for the `web` service (UI + API proxy).                                                   |
| `REDIS_URL`        | Redis connection URL                                             | `redis://localhost:6379/0`<br> compose: `redis://redis:6379/0` | Storage url.                                                                                                      |
| `SHUTDOWN_TIMEOUT` | Go duration, e.g. `30s`                                          | `10s`                                                          | Max time to gracefully stop in-flight requests on shutdown. Keep below the deployment's termination grace period. |
| `LOG_LEVEL`        | `debug` `info` `warn` `error`                                    | `info`                                                         | Minimum log level being printed.                                                                                  |
| `LOG_FORMAT`       | `text` `json`                                                    | `text`                                                         | `text` is a human readable format (colorized if using stdout), `json` is for machines.                            |
| `LOG_FILE`         | file path                                                        | *(empty → stdout)*                                             | If set, logs go to this file only.                                                                                |
| `LOG_TIME`         | `sec` `milli` `nano` `dt-sec` `dt-milli` `rfc3339` `rfc3339nano` | `dt-milli`                                                     | timestamp layout; `dt-` means the date is printed. Does not affect `LOG_FORMAT = json` (always RFC3339Nano).      |

For example,

```bash
LOG_LEVEL=debug LOG_FORMAT=text LOG_FILE=./apex.log go run .
```

will be logging messages like `2026-08-03 05:23:40.123 INFO starting server addr=:8090` into a file.

Note: `LOG_LEVEL=debug` is a diagnostic mode, not a production setting.
In particular, it will log every Redis command with its arguments, duration and the result.

Envs can be overriden for `docker compose` runs by exporting them in the
shell or via an `.env` file. See `/.env.template`.

Example:

```bash
docker compose --env-file .env.dev up
```

### API Walk

Exercises every endpoint against a running server:

```bash
go run ./apiscripts/apiwalk -base-url http://localhost:8090 -board demo-cup
```

### Run Tests

```bash
go test ./...                    # unit tests
go test -tags=integration ./...  # unit + integration tests with db (uses Docker)

go test -tags=integration -run TestStorageSuite ./storage/  # integration tests only
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

## Misc

No connection with the game Apex Legends whatsoever.

Apex Legends™ is a trademark of Electronic Arts Inc.
