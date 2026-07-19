# Apex

Apex is the backend web service implementing the Leaderboard functionality. A lightweight HTTP API for storing and retrieving player scores using Redis (inspired by https://redis.io/solutions/leaderboards/).

## 🚀 Quick Start

You only need [Docker](https://docs.docker.com/get-docker/). Run the app:

```bash
docker compose up --build      # app on :8090, Redis on :6379
```

Make your first requests against `:8090`:

```bash
# Create a player with an initial score
curl -X POST http://localhost:8090/api/v1/scores \
  -H "Content-Type: application/json" \
  -d '{"player_name":"alice","player_score":42.5}'
# {"player_id":"7dcbeb46-e1e1-492d-a32a-c593b13428de"}

# Fetch that player back by its id
curl http://localhost:8090/api/v1/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de
# {"player_id":"7dcbeb46-...","player_name":"alice","player_score":42.5}
```

## 🛠️ Development

Working on the code needs 1.26+ [Go](https://go.dev/doc/install) in addition to Docker.

### Running the Server

**Everything in Docker (app + Redis)** — uses [`docker-compose.yml`](docker-compose.yml)

```bash
docker compose up -d --build   # app on :8090, Redis on :6379 (data persisted in a volume)
docker compose logs -f app     # follow the app logs
docker compose down            # stop the stack (add -v to wipe Redis data)
```

**Locally with Go (Redis in Docker)**

```bash
docker compose up -d redis 
# or just docker: docker run -p 6379:6379 redis:8.8.0-alpine
go run .
```

The server listens on port `:8090` and connects to Redis via `REDIS_URL`
(default `redis://localhost:6379/0`). Point it elsewhere with the env var:

```bash
REDIS_URL=redis://:password@host:6379/0 go run .
```

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
# Create a player; returns the generated id
curl -X POST http://localhost:8090/api/v1/scores \
  -H "Content-Type: application/json" \
  -d '{"player_name":"alice","player_score":42.5}'
# {"player_id":"7dcbeb46-e1e1-492d-a32a-c593b13428de"}

# Fetch a player by id
curl http://localhost:8090/api/v1/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de
# {"player_id":"7dcbeb46-...","player_name":"alice","player_score":42.5}

# Increment player's score
curl -X POST http://localhost:8090/api/v1/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de/increment \
  -H "Content-Type: application/json" \
  -d '{"amount":5}'
# {"score":47.5}

# Set an absolute score
curl -X PUT http://localhost:8090/api/v1/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de \
  -H "Content-Type: application/json" \
  -d '{"player_score":100}'

# List the leaderboard, highest first (top 10 by default; page with ?limit= & ?offset=)
curl "http://localhost:8090/api/v1/scores?limit=10&offset=0"
# {"scores":[{"player_id":"7dcbeb46-...","score":100,"rank":1}],"limit":10,"offset":0,"total":1}

# A single player's standing (rank is 1-based; total is the board size)
curl http://localhost:8090/api/v1/scores/7dcbeb46-e1e1-492d-a32a-c593b13428de/rank
# {"player_id":"7dcbeb46-...","rank":1,"score":100,"total":1}
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

Run `go mod tidy` to sync `go.mod` and `go.sum` with the actual imports.

```bash
go mod tidy
```

### Developer Docs

- [`api.yaml`](api.yaml) - OpenAPI specification
- See docs/ folder. In particular:
  - [docs/architecture.md](docs/architecture.md) - how the system is put together and why
  - [docs/design.md](docs/design.md) - vocabulary and invariants
  - [docs/tests.md](docs/tests.md) - testing approach

## Misc

No connection with the Apex Legends game whatsoever.
