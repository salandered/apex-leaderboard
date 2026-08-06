# Scripts

Independent Go module.

## Apiwalk

```bash
go run ./apiwalk [-base-url http://localhost:8090] [-board demo-cup]
```

## LoadScores

_Developer scripts_

Fires concurrent increment requests at a running apex instance and verifies the final score.

```bash
go run ./loadscore [-base-url http://localhost:8090] [-requests 1000] [-amount 1]
```

## Fanout

Fans many players out onto one board with distinct scores concurrently, then verifies the
leaderboard ranks them correctly (contiguous ranks, listing vs single-standing agree, paging across
the 100-row seam) and the projection stays clean.

```bash
go run ./fanout [-base-url http://localhost:8090] [-players 200] [-chunk-size 25]
```

## Daily activity

Creates new players, writes distinct event counts for the current UTC day, then waits for the
eventually consistent activity projection and verifies their counts and relative order.

```bash
go run ./dailyactivity [-base-url http://localhost:8090] [-timeout 15s]
```

## Random history

Creates one board and one player, then writes a long run of random score events (mostly small
increments) and verifies the final score and that the returned ledger matches the events written, in order.

```bash
go run ./randomhistory [-base-url http://localhost:8090] [-events 500] [-seed 0] [-board ""]
```

`-board` writes to that board id, creating it if missing, so several runs can pile players onto one
board; the default creates a run-scoped one.

## Config

Run `go run ./<script> --help` for a specific script.

## Tool runner image

The Dockerfile builds every `package main` below this directory. The executable name
is the command directory name. The `toolrunner` command itself is excluded.

Build from the repository root, using `apiscripts` as the build context:

```bash
docker build -t apex-tools ./apiscripts
```

Run the service:

```bash
docker run --rm -p 127.0.0.1:8091:8091 apex-tools
```

List the compiled tools:

```bash
curl http://localhost:8091/dev/tools
```

Run one tool. Arguments are passed directly to the executable without a shell:

```bash
curl -X POST http://localhost:8091/dev/tools/apiwalk \
  -H "Content-Type: application/json" \
  -d '{"args":["-base-url","http://app:8090","-board","demo-cup"]}'
```

Only one tool runs at a time. `TOOL_TIMEOUT` controls the default 30-sec execution timeout.
