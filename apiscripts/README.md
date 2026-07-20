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
cd apiscripts
go run . -base-url=http://localhost:8090 -requests=1000 -amount=1
```

## Config

Run `go run . --help` for a specific script.
