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

Creates fresh players, writes distinct event counts for the current UTC day, then waits for the
eventually consistent activity projection and verifies their counts and relative order.

```bash
go run ./dailyactivity [-base-url http://localhost:8090] [-timeout 15s]
```

## Config

Run `go run ./<script> --help` for a specific script.
