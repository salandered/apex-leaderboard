# Current design desicions

## Language (aiming for this)

| Term              | Means                                                          | Context                   |
| ----------------- | -------------------------------------------------------------- | ------------------------- |
| **event**         | one applied operation (a fact)                                 | all                       |
| **ledger**        | append-only record of score **events**                         | all                       |
| **tombstone**     | the delete **event**                                           | app, API uses "delete"?   |
| **board**         | named score container with lifecycle.                          | all                       |
| projection        | content of the **board** (derived view of the **ledger**)      | app                       |
| standing          | **projection** read model: (playerId, boardId, value, rank)    | API uses generic "score"? |
| **replay**        | Build a **projection** using the **ledger**                    | app                       |
| idempotency table | idempotency records: what reqIds were applied using **events** | app                       |
| **profile**       | player's info (no score)                                       | all                       |
| **stream entry**  | Redis Stream item (raw **event**)                              | redis                     |

Contexts:

* all (codebase, docs, public API)
* app (codebase, docs)
* redis (storage codebase, not domain)

## Rules (invariants)

1. **The event stream is the source of truth for standings.** The leaderboard ZSET (Sorted Set) is a projection:
   it can be deleted and rebuilt from stream, and the result must be the same.

2. **Events record facts only.** An event exists iff the operation was applied.
   E.g failed score increment is not appended.

3. **A non idempotent write carries a client `request_id` (Idempotency-key).**
   Retrying the same request_id produces the same result.

4. **`set` is a snapshot barrier.** The current score of a player is:
   `last set value + sum of increments after it`. Replay never needs to look
   past the most recent `set`.

## Storage API notes

### Increment endpoint (`POST scores/{id}/increment`) and Idempotency key

Problem: `increment` applies a delta (`+amount`) which is not idempotent.
If a client retries, we'll end up with a wrong, doubled value. **This makes it barely usable**.
Fix: making endpount actually idempotent via a unique key.

The client attaches a unique token. The server applies the increment and marks a token as "served" (with some TTL). A later request won't apply the operation.

Using the header is a known implementation, we do the same.

* https://docs.stripe.com/api/idempotent_requests
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Idempotency-Key

Lua script is used to ensure that an operation is atomic.

*Note: alternative is to remove `/increment` and use set. But that introduces the worse get + set race condition, see info below.*

### Set endpoint (`PUT /api/v1/boards/{board_id}/scores/{player_id}`)

Potential problem: read-modify-write, a client does `GET` → computes a new value → `PUT`.
Two clients might read the same value and then set different values, one of them will be lost.

This depends on a client scenario and covers the usage of the **two endpoints together**.
*Do we need this for a leaderboard?* Scores are volatile, and usually are incremented (and if the absolute value is set, it does not depend on the previous value). Also if a race occurs, last-write-wins may be fine.

#### Solution to consider: OCC

GET returns a version alongside the value, and have the client send it back on PUT.
The server applies the write only if the version still matches.

Implementation notes:

* Where to store and how to maintain the version in redis
* Lua script will be needed
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/ETag
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/If-Match

### Listing endpoint (`GET /api/v1/boards/{board_id}/scores`) and pagination

The leaderboard is a ZSET which naturally is good for any kind of ranges and hence the pagination. Arguably most important leaderboard functionality - Top N - is just the pagination with starting with the first page.

Two important pagination properties:

* **Performance** — not an issue for us. Sorted set performs ranges in log time
(In SQL or mongo offset is not cheap and usually cursor is used)
* **Consistency** — under concurrent writes items shift between page reads,
so some items can be skipped or duplicated.

#### Approaches

**Offset / limit** — `?limit=10&offset=0`

Pros: simplest, no state, the native ZSET operation.

Cons: weakest consistency.

**Cursor relies on player_id** — cursor = the last row's `(score, player_id)`.

Next page: `rank = ZREVRANK(leaderboard, player_id)`, then `ZREVRANGE rank+1 rank+size`.

Pros:

* also simple, while lua scripting is probably required
* more robust than offset — unaffected by changes far above the cursor.
  misses. (The `score` half is only a staleness check, or a fallback if delete ever returns.)

Cons:

We rely on the score. If the anchor's score moves,
even an *unchanged* row can be skipped or duplicated.

Illustration with page size 2, and elements `A=100 B=90 C=80 D=70`:

* page 1 returns `A, B`, cursor = `B`.
* `B` drops 90 → 75 => new order `A, C, B, D`
* page 2 reads `D`
* => `C` is skipped and never was returned

**Value cursor (composite score)** — cursor = the last row's score value.

Next page op will look something like `ZRANGE key (S_last -inf REV BYSCORE LIMIT 0 size`.

* The `(` bound is exclusive, so scores must be **unique**.
* Solution: bake a tiebreaker into the score (e.g. `points·BIG + seq`, or an inverse
  timestamp in the fraction), so no two members collide.

Pros: the "correct" cursor — anchors on a *fixed value* in the total order, so every
  *stationary* row is returned exactly once.
  
Cons:

* Complex implementation: changes how score are stored (more code, less transparent db etc)
* without unique scores the exclusive bound `(` skips tied members

#### Decision

Currenly using the simplest one. The second approach is a bit weird,
and the third one is complex and the pay off is unclear.
