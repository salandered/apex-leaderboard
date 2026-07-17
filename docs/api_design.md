# Storage API notes

## Increment endpoint (`POST scores/{id}/increment`) and Idempotency key

Problem: `increment` applies a delta (`+amount`) which is not idempotent.
If a client retries, we'll end up with a wrong, doubled value. **This makes it barely usable**.
Fix: making endpount actually idempotent via a unique key.

The client attaches a unique token. The server applies the increment and marks a token as "served" (with some TTL). A later request won't apply the operation.

Using the header is a known implementation, we do the same.

* https://docs.stripe.com/api/idempotent_requests
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Idempotency-Key

Lua script is used to ensure that an operation is atomic.

_Note: alternative is to remove `/increment` and use set. But that introduces the worse get + set race condition, see info below._

## Set endpoint (`PUT /api/v1/scores/{id}`)

Potential problem: read-modify-write, a client does `GET` → computes a new value → `PUT`.
Two clients might read the same value and then set different values, one of them will be lost.

This depends on a client scenario and covers the usage of the **two endpoints together**.
_Do we need this for a leaderboard?_ Scores are volatile, and usually are incremented (and if the absolute value is set, it does not depend on the previous value). Also if a race occurs, last-write-wins may be fine.

### Solution to consider: OCC

GET returns a version alongside the value, and have the client send it back on PUT.
The server applies the write only if the version still matches.

Implementation notes:

* Where to store and how to maintain the version in redis
* Lua script will be needed
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/ETag
* https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/If-Match
