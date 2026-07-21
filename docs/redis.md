
# Working with Redis

- [Client library (go-redis)](#client-library-go-redis)
- [Reading results from a pipeline](#reading-results-from-a-pipeline)
	- [`redis.Nil` cheatsheet](#redisnil-cheatsheet)
- [Lua scripting](#lua-scripting)
	- [Theory](#theory)
	- [Example using go-redis](#example-using-go-redis)
	- [Misc](#misc)
- [Redis streams](#redis-streams)
- [Developer notes](#developer-notes)

## Client library (go-redis)

- [go-redis guide](https://redis.uptrace.dev/)
- [go-redis package docs](https://pkg.go.dev/github.com/redis/go-redis/v9)

## Reading results from a pipeline

See https://redis.uptrace.dev/guide/go-redis-pipelines.html about how pipelining works.

Important note is that some commands return error `redis.Nil` ([link](https://redis.uptrace.dev/guide/go-redis.html#redis-nil)) on a missing key.
Since a missing key may be an expected outcome, we need to separate a real error and `redis.Nil`.

This leads to a pattern which is used in the codebase:

```go
pipe := client.Pipeline()
aCmd := pipe.ZCard(ctx, key) 
bCmd := pipe.ZRevRankWithScore(ctx, key, member)

//  Check Exec for transport-level failure. Filter redis.Nil if some queued
//  command can return it (e.g. ZSCORE on a missing member).
if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
    return err
}

//  Read the command with Result() and separate redis.Nil if it's an expected behaviour.
a, err := aCmd.Result() 
if err != nil { ... }
b, err := bCmd.Result()
if errors.Is(err, redis.Nil) { ... } // expected: not found
if err != nil { ... }
```

Some other things to notice here:

`Exec` returns `([]redis.Cmder, error)`, where the slice holds the
*same* command objects that we queued (`aCmd`, `bCmd`). So it can be discurded (`_`).

`Exec`'s error is the **first** error among the queued commands. It won't tell you *which* command failed.

Results like `aCmd.Result()` are only available after `pipe.Exec` was called.

When no queued command can return `redis.Nil`, `_, err := pipe.Exec(ctx); err != nil`
is enough and per-command checks are redundant.
Codebase still uses them so every pipeline reads the same way.

This leads to that we never use `Cmd.Val()` (returns the zero value on error instead of `(value, error)`).

### `redis.Nil` cheatsheet

| Command                | Missing key/member yields   |
| ---------------------- | --------------------------- |
| `ZSCORE`, `ZREVRANK`   | `redis.Nil`                 |
| `HGETALL`              | empty map (NOT `redis.Nil`) |
| `ZCARD`                | `0`                         |
| `ZRANGE` / `ZREVRANGE` | empty slice                 |
| `XRANGE` / `XREVRANGE` | empty slice                 |

## Lua scripting

Redis lets you run Lua scripts server-side via `EVAL`.
They provie **atomicity**, the script runs as one uninterrupted operation.

See links:

- https://redis.io/docs/latest/develop/programmability/eval-intro/\
- https://redis.uptrace.dev/guide/lua-scripting.html

### Theory

```lua
EVAL "return redis.call('SET', KEYS[1], ARGV[1])" 1 mykey myvalue
```

- `redis.call` - calling Redis command
- `KEYS[]` - Redis keys the script touches
- `ARGV[]` - plain arguments
- `1` - number of keys following
- `mykey` - the actual key, accessible in the script as `KEYS[1]`

### Example using go-redis

Atomic read-decide-write increment:

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or "0")
if current < 100 then
    return redis.call('INCR', KEYS[1])
else
    return current
end
```

Running from Go:

```go
script := redis.NewScript(`<lua-script>`)

result, err := script.Run(ctx, rdb, []string{"mycounter"}).Result()
```

For more information see project lua scripts.

### Misc

Lua language extensions might warn you with messages like "Undefined global `ARGV`"

For example, this [VSCode extension](https://marketplace.visualstudio.com/items?itemName=sumneko.lua) warnings can be supressed in settings:

```json
"Lua.diagnostics.globals": [
		"redis",
		"KEYS",
		"ARGV"
	],
```

## Redis streams

Background for the event-sourced score model (the ledger).

A stream is an append-only data strucute.

ℹ️ Official docs refer to the stream item as a "stream entry" (not event or message).
So we use the same term for describing the raw event (in docs and the codebase)

Each entry has an auto-assigned id (if using `*`) `<unix_ms>-<seq>` and a flat list of field/value pairs.

```
XADD ledger:events * type set player_id alice amount 0 request_id r1
```

Reading a range (`-` = min id, `+` = max id):

```json
XRANGE ledger:events - +
1) 1) "1752771000000-0"
   2) 1) "type"        2) "set"
      3) "player_id"   4) "alice"
      5) "amount"      6) "0"
      7) "request_id"  8) "r1"
```

Other basic commands:

- `XLEN ledger:events` - number of entries.
- `XREVRANGE ledger:events + - COUNT 10` - newest first.
- `XRANGE ledger:events <last_seen_id> +` - resume reading from a checkpoint
  (this is how `Rebuild` or an async consumer makes progress).
- `XREAD COUNT 10 BLOCK 5000 STREAMS ledger:events 0-0` - blocking read for 10 entries after the `0-0` cursor.
- Consumer groups (`XGROUP` / `XREADGROUP`) exist for *asynchronous* projections with
  acking.

## Developer notes

Clear (!) the database: `docker compose exec redis redis-cli FLUSHALL`

List all events in the stream: `XRANGE ledger:events - +`
