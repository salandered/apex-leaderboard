--[[
	TODO: consider merging with the other lua script

	
	Performs an atomic score write:
		- dedupe check
		- apply to projection
		- append event
		- record request_id (idempotency key)

	KEYS[1] = projection zset      (leaderboard)
	KEYS[2] = event stream         (score:events)
	KEYS[3] = idempotency hash     (score:applied)
	
	ARGV[1] = operation type       ("set" | "increment")
	ARGV[2] = player_id            (UUID)
	ARGV[3] = amount               (float)
	ARGV[4] = request_id           (UUID)
	
	Returns: { applied (1|0), new_score (string), stream_id }

	The caller is responsible for request validation (e.g. rejecting an increment
	of an unknown player); by the time this script runs the write is accepted, so
	every invocation that gets past the dedupe check appends exactly one event.
]]

local zset_key, stream_key, applied_key = KEYS[1], KEYS[2], KEYS[3]
local op_type, player_id, amount, req_id = ARGV[1], ARGV[2], ARGV[3], ARGV[4]

-- Dedupe check: same request_id -> no-op, return current value.
local seen = redis.call('HGET', applied_key, req_id)
if seen then
	local cur = redis.call('ZSCORE', zset_key, player_id) or '0'
	return { 0, cur, seen }
end

-- Apply to the projection.
local newscore
if op_type == 'set' then
	redis.call('ZADD', zset_key, amount, player_id)
	newscore = amount
else
	newscore = redis.call('ZINCRBY', zset_key, amount, player_id)
end

-- Append the fact to the ledger and remember the request_id.
local id = redis.call(
	'XADD', stream_key, '*',
	'type', op_type,
	'player_id', player_id,
	'amount', amount,
	'request_id', req_id
)

redis.call('HSET', applied_key, req_id, id)

return { 1, tostring(newscore), id }
