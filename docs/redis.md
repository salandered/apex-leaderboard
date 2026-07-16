
# Working with Redis

## Client library (go-redis)

- [go-redis guide](https://redis.uptrace.dev/) — overview and getting started
- [go-redis API reference](https://pkg.go.dev/github.com/redis/go-redis/v9) — package docs (pkg.go.dev)

## Some topics

- [Handling `redis.Nil`](https://redis.uptrace.dev/guide/go-redis.html#redis-nil) — detecting missing keys
- [Typed errors](https://github.com/redis/go-redis/tree/master#typed-errors) — error types to check against
- [Pipelines](https://redis.uptrace.dev/guide/go-redis-pipelines.html) — batching commands

## Local dev

`docker compose exec redis redis-cli FLUSHALL`
