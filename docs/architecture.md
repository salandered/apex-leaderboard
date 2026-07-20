# Architecture

Apex is a leaderboard backend: a Go HTTP service with Redis as the only datastore. Clients manage
player profiles and boards, write scores, and read rankings through a JSON API described by OpenAPI spec.

## The core idea: event sourced score

Every score change is recorded as an **event** in an append-only **ledger** (a Redis Stream).
The ledger is the source of truth for the score values. The leaderboards are **projections**:
derived views that can be deleted and rebuilt from the ledger with an identical result.
Each leaderboard is a Redis Sorted Set.

Pros:

- a full audit history of every score (the history API is just a ledger read)
- disposable rankings - projection corruption is repaired by replay
- leaderboards projections (essentially secondary B-tree indexes) allow log times for range operations
- future views (weekly boards, "biggest gainer") as new consumers of the same stream, with no
  changes to write operations

The cost:

- every write goes through a Lua script (provides the strongest transactional guarantee Redis offers) touching several keys.
  This leads to a less transparent core db operations, and, most importantly, ties the design to a single Redis node
  (fine untill we want a Redis Cluster).
- the stream grows unboundedly - trimming is forbidden until a snapshotting scheme exists.

## Components

<!-- diagram: data model (profiles, boards, ledger, projections) -->

**Player profiles.** Global, board-independent documents (name, creation date) keyed by a
server-generated UUID. Creating a player is profile-only: a player can exist with no scores.

**Boards.** Named score containers. Ids are short, client-chosen slugs (`summer-contest2026`)
rather than UUIDs. They are readable and appear in URLs.
The board id is **immutable forever** (ids are written into ledger events),
however, a board has a mutable display name.
A registry (currently acts as a sorting index) keeps the list of boards in creation order.
The default board `main` is created at startup.
Boards carry an `active`/`closed` status: a closed
board rejects score writes with `409` while reads and ledger replay are unaffected.
In particualr, a closed board allows to rebuild the leaderboard projection from the ledger without racing with
concurrent new score writes.
Board can be reopened. Currently boards cannot be deleted.

**The ledger.** One global stream containing all score events.
Event is recorded only if the operation was succesfully applied (fact only).
Currently two event types exist: `set` and `increment` (a delta).
"Set" typed event acts as a snapshot barrier - replay never needs to look past the latest `set`.

**Projections.** The actual leaderboards which face clients. One sorted set per board holding the current scores.
In app (not API) we call a projection entry a **standing**, because besides the score value it holds a player id
and also implicitly implies a "rank" - its index (1-based). So standing is a (score, player_id, rank). All standing reads -
top-N pages, a single player's standing - are cheap sorted-set operations. It allows listing operations use plain
limit/offset pagination.

**Idempotency hash.** Every write records a server-generated request id in its event.
A client might send an optional `Idempotency-Key` header: the write would store
a fingerprint (`entry_id|op|amount`) under that key with a TTL. This makes retries idempotent
(essential for the incrementing a score op).
The same key reused with a different op/amount is rejected with `409`. Score writes return
`204` (no body).

Player creation has its own idempotency (separate hash). A repeated replays posts nothing and returns
the same generated `player_id` or `409`s.

Board creation doesnt use this mechanics: `PUT` with a client-chosen slug is already retry-safe.

## The write operations

<!-- diagram: write path (script: dedupe -> checks -> apply -> append -> record) -->

Every score write runs one Lua script executing atomically: optional idempotency check → player and
board existence check → apply to the projection → append the event → record the idempotency key
(only when the client supplied one). Projection and ledger move together or not at all.

Rebuild and verification are the operational counterpart. Both are scoped to one board.
Rebuild folds board's ledger events into its projection (a leaderboard). Verification does the same but with a
scratch and then compares it with a live projection.

## How it got here

The design went through three stages:

1. **Hash + sorted set.** Player profiles are in hashes, scores in a sorted set. Player to score 1 to 1.
    One leaderboard. Inspired by (https://redis.io/solutions/leaderboards/).
2. **Event sourced scores.** The ledger became the source of truth and the sorted set a projection;
   scores gained history and idempotent writes.
3. **Multi-board.** Board became a first-class object. Board to a leaderboard projection 1 to 1.
   Player to scores 1 to N.
