# loadtest

Fires concurrent increment requests at a running apex instance and verifies the final score.

Independent Go module (own `go.mod`), so its dependencies (resty) never touch the main module.

## Run

```
cd loadtest
go run . -base-url=http://localhost:8090 -requests=1000 -amount=1
```

Flags:

- `-base-url` — apex service URL (default `http://localhost:8090`)
- `-requests` — number of increment requests to send (default `1000`)
- `-amount` — amount added per request (default `1`)
- `-chunk-size` — requests launched together before waiting `-chunk-delay` (default `50`)
- `-chunk-delay` — delay between chunks (default `20ms`)
