
# Working with Redis

## Client library (go-redis)

- [go-redis guide](https://redis.uptrace.dev/) — overview and getting started
- [go-redis API reference](https://pkg.go.dev/github.com/redis/go-redis/v9) — package docs (pkg.go.dev)

## Some topics

- [Handling `redis.Nil`](https://redis.uptrace.dev/guide/go-redis.html#redis-nil) — detecting missing keys
- [Typed errors](https://github.com/redis/go-redis/tree/master#typed-errors) — error types to check against
- [Pipelines](https://redis.uptrace.dev/guide/go-redis-pipelines.html) — batching commands

## Recipe: reading results from a pipeline

Queuing a command on a pipeline returns a typed handle (`*IntCmd`, `*ZSliceCmd`, …); results are
not available until `Exec` runs. `Exec` returns `([]redis.Cmder, error)` — the slice holds the
*same* command objects you already queued, so it is safe to discard (`_`) when you kept the typed
handles. Two things matter:

- `Exec`'s error is the **first** error among the queued commands, and `redis.Nil` counts as an
  error. It cannot tell you *which* command failed, and it masks any later command's error.
- `cmd.Val()` returns the zero value on error; `cmd.Result()` returns `(value, error)`.

The uniform pattern used in this codebase:

```go
pipe := client.Pipeline()
aCmd := pipe.ZCard(ctx, key)
bCmd := pipe.ZRevRankWithScore(ctx, key, member)

// 1. Check Exec for transport-level failure. Filter redis.Nil ONLY if some queued
//    command can legitimately return it (e.g. ZSCORE / ZREVRANK on a missing member).
if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
    return err
}

// 2. Read every command with Result() and check its error individually — this is
//    where an expected redis.Nil is told apart from a real failure.
a, err := aCmd.Result()
if err != nil { ... }
b, err := bCmd.Result()
if errors.Is(err, redis.Nil) { ... } // expected: not found
if err != nil { ... }
```

When no queued command can return `redis.Nil`, step 1 alone is exhaustive and the per-command
checks are redundant — they are kept anyway so every pipeline reads the same way.

Which reads produce `redis.Nil` vs an empty value (missing key/member):

| Command                | Missing key/member yields   |
| ---------------------- | --------------------------- |
| `ZSCORE`, `ZREVRANK`   | `redis.Nil`                 |
| `HGETALL`              | empty map (NOT `redis.Nil`) |
| `ZCARD`                | `0`                         |
| `ZRANGE` / `ZREVRANGE` | empty slice                 |
| `XRANGE` / `XREVRANGE` | empty slice                 |

## Local dev

`docker compose exec redis redis-cli FLUSHALL`
