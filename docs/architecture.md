# Architecture

Apex is a leaderboard backend: a Go HTTP service with Redis as the only datastore. Clients manage
player profiles and boards, write scores, and read rankings through a JSON API described by OpenAPI spec. This document explains how the system is put together and why.

## The core idea: event sourcing

Every score change is recorded as an **event** in an append-only **ledger** (a Redis Stream).
The ledger is the source of truth. The leaderboards are **projections**: derived views that can be deleted and rebuilt from the ledger with an identical result. Each leaderboard is a Redis Sorted Set.

Pros:

- a full audit history of every score (the history API is just a ledger read)
- disposable rankings - projection corruption is repaired by replay (not restored from backup, for example)
- such leaderboards projections (essentially secondary B-tree indexes) - allow log times for range operations out-of-the-redis-box
- future views (weekly boards, "biggest gainer") as new consumers of the same stream, with no
  changes to write operations
- retry-safe writes (see idempotency below)

The cost:

- every write goes through a Lua script (provides the strongest transactional guarantee Redis offers) touching several keys. This leads to a less transparent core db operations, and, most importantly, ties the design to a single Redis node (fine at this scale, will be a problem with a Redis Cluster).
- the stream grows unboundedly - trimming is forbidden until a snapshotting scheme exists.

## Components

<!-- diagram: data model (profiles, boards, ledger, projections) -->

**Player profiles.** Global, board-independent documents (name, creation date) keyed by a
server-generated UUID. Creating a player is profile-only: a player can exist with no scores.
There is no  "join a board" operation - a player's first score write on a board
enrolls them there.

**Boards.** Named score containers. Ids are short, client-chosen slugs (`summer-contest2026`)
rather than UUIDs. They are readable and appear in URLs. For simplicity the board id is **immutable forever** (ids are written into ledger events), however, a mutable display name lives
alongside. A registry keeps the list of boards in creation order. The default board `main` is
created at startup.
Boards currently cannot be closed or deleted; a lifecycle (open/closed status) is a planned extension.

**The ledger.** One global stream for all boards containing events like a score increment.
Event is recorded only if the operation was succesfully applied (fact-only).
Two event types exist: `set` (absolute
value, also a snapshot barrier - replay never needs to look past the latest `set`) and
`increment` (a delta).

**Projections.** One sorted set per board holding the current scores. We call a projection entry a **standing**, because besides the score value it explicitly holds a player id and implicitly a "rank",
which is just its index (1-based), so standing is a (score, player_id, rank). All ranking reads -
top-N pages, a single player's rank - are cheap sorted-set operations. Listing uses plain
limit/offset pagination.

**Idempotency (applied table).** Non-idempotent writes (`increment`) carry a request id via a
`Idempotency-Key` header; the write operation records each applied id and turns a retry into a no-op returning the original result.

## The write operation

<!-- diagram: write path (script: dedupe -> checks -> apply -> append -> record) -->

Every score write runs one Lua script executing atomically: dedupe check → player and board
liveness checks → apply to the projection → append the event → record the request id. Projection
and ledger move together or not at all. See also [lua_scripting.md](lua_scripting.md).

Replay and verification are the operational counterpart: an admin-side rebuild folds the ledger
into fresh projections, and a verify pass replays it into a scratch key and diffs it against the
live boards.

## How it got here

The design went through three stages, each replacing the previous one's central assumption:

1. **Hash + sorted set.** Profiles in hashes, one global leaderboard sorted set, kept bijective:
   every player had exactly one score. The sorted set *was* the data.
2. **Event sourcing.** The ledger became the source of truth and the sorted set a projection;
   scores gained history and idempotent writes.
3. **Multi-board.** Boards became first-class: per-board projections over the same single
   ledger, profiles went global, and the bijection dissolved - a (board, player) score exists
   iff the ledger says so.
