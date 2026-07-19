# Redis Lua scripting

- [Useful links](#useful-links)
- [Quick start](#quick-start)
	- [Accessing Redis commands from Lua](#accessing-redis-commands-from-lua)
	- [KEYS vs ARGV](#keys-vs-argv)
	- [Example with atomic increment](#example-with-atomic-increment)
	- [Calling from Go](#calling-from-go)
- [Tips](#tips)

Redis lets you run Lua scripts server-side via `EVAL`.
The main reason to use it: **atomicity** — the whole script runs as one uninterruptible operation,
no other client's commands can interleave.

## Useful links

- https://redis.io/docs/latest/develop/programmability/eval-intro/\
- https://redis.uptrace.dev/guide/lua-scripting.html

## Quick start

```lua
EVAL "return 'hello'" 0
```

`0` here means "0 keys passed in".

### Accessing Redis commands from Lua

Use `redis.call()`:

```lua
EVAL "return redis.call('GET', KEYS[1])" 1 mykey
```

- `1` = number of keys following
- `mykey` = the actual key, accessible in the script as `KEYS[1]`

### KEYS vs ARGV

- `KEYS[]` — Redis keys the script touches (lets Redis Cluster route correctly)
- `ARGV[]` — plain arguments, not treated as keys

```lua
EVAL "return redis.call('SET', KEYS[1], ARGV[1])" 1 mykey myvalue
```

### Example with atomic increment

Say you want to increment a counter, but only if it stays under 100:

```lua
local current = tonumber(redis.call('GET', KEYS[1]) or "0")
if current < 100 then
    return redis.call('INCR', KEYS[1])
else
    return current
end
```

For simplicity this code snippet will be reffered as <lua-script>.

```bash
EVAL "<lua-script>" 1 mycounter
```

Without Lua, doing "check then increment" from your app code has a race condition — two clients could both read `99` and both increment.

### Calling from Go

Using `go-redis`:

```go
script := redis.NewScript(`<lua-script>`)

result, err := script.Run(ctx, rdb, []string{"mycounter"}).Result()
```

## Tips

It's ok if a Lua language extensions warns you with messages like "Undefined global `ARGV`"

For example, this [VSCode extension](https://marketplace.visualstudio.com/items?itemName=sumneko.lua) warnings can be supressed in settings:

```json
"Lua.diagnostics.globals": [
		"KEYS",
		"redis",
		"ARGV"
	],
```
