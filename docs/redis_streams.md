# Redis Streams

Background for the event-sourced score model (the ledger).

- [Streams quickstart](#streams-quickstart)
- [A worked sequence](#a-worked-sequence)

## Streams quickstart

A stream is an append-only data strucute.

ℹ️ Official docs refer to the stream item as "stream entry".
We use this term in docs and the codebase as well.

Each entry has an auto-assigned id (if using `*`) `<unix_ms>-<seq>` and a flat list of field/value pairs.

```
XADD ledger:events * type set player_id alice amount 0 request_id r1
```

Reading a range (`-` = min id, `+` = max id):

```
XRANGE ledger:events - +
1) 1) "1752771000000-0"
   2) 1) "type"        2) "set"
      3) "player_id"   4) "alice"
      5) "amount"      6) "0"
      7) "request_id"  8) "r1"
```

Other commands this service relies on:

- `XLEN ledger:events` — number of entries.
- `XREVRANGE ledger:events + - COUNT 10` — newest first (history endpoint).
- `XRANGE ledger:events <last_seen_id> +` — resume reading from a checkpoint
  (this is how `Rebuild` or an async consumer makes progress).
- Consumer groups (`XGROUP` / `XREADGROUP`) exist for *asynchronous* projections with
  acking. This service doesn't use them — the projection is updated synchronously in the
  same Lua script. They become relevant only if a second projection is added (e.g. per-tag
  boards fed from the same stream).

## A worked sequence

Sequence for player `alice`: `set(0) +3 +10 set(50) -4`.

Each line below is one call of the write script (shown as `EVAL` for clarity,
`redis.NewScript` does this). The three keys are the projection ZSET
`leaderboard`, the ledger stream `ledger:events`, and the idempotency hash
`ledger:idempotency`; the returned triple is `{ applied, new_score, stream_id }`.

```
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency set       alice  0  r1
→ 1) 1   2) "0"   3) "1752771000001-0"
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency increment alice  3  r2
→ 1) 1   2) "3"   3) "1752771000002-0"
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency increment alice 10  r3
→ 1) 1   2) "13"  3) "1752771000003-0"
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency set       alice 50  r4
→ 1) 1   2) "50"  3) "1752771000004-0"
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency increment alice -4  r5
→ 1) 1   2) "46"  3) "1752771000005-0"
```

State afterwards:

```
ZSCORE leaderboard alice      → "46"       (projection: current standing)
XLEN ledger:events             → 5          (ledger: full history)
```

Retry safety — the client resends `r5` after a timeout:

```
EVAL "<script>" 3 leaderboard ledger:events ledger:idempotency increment alice -4  r5
→ 1) 0   2) "46"  3) "1752771000005-0"
```

`applied=0`, score unchanged, original stream id returned.

History, newest first:

```
XREVRANGE ledger:events + - COUNT 3
1) 1752771000005-0  type=increment  player_id=alice  amount=-4  request_id=r5
2) 1752771000004-0  type=set        player_id=alice  amount=50  request_id=r4
3) 1752771000003-0  type=increment  player_id=alice  amount=10  request_id=r3
```
