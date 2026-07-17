--[[
	Performs an atomic player create:
		- dedupe check
		- write the profile hash
		- apply the initial score to the projection
		- append a `set` event
		- record request_id (idempotency key)

	Create is a `set` that also writes the profile, so profile and score can never
	drift apart (no dual write). Sibling of apply_score_event.lua, which owns every
	later score mutation.

	KEYS[1] = projection zset      (leaderboard)
	KEYS[2] = event stream         (score:events)
	KEYS[3] = idempotency hash     (score:applied)
	KEYS[4] = profile hash         (player:{id})

	ARGV[1] = player_id            (UUID)
	ARGV[2] = player_name
	ARGV[3] = score                (float, absolute)
	ARGV[4] = request_id           (UUID)

	Returns: { applied (1|0), new_score (string), stream_id }
]]

local zset_key, stream_key, applied_key, profile_key = KEYS[1], KEYS[2], KEYS[3], KEYS[4]
local player_id, player_name, score, req_id = ARGV[1], ARGV[2], ARGV[3], ARGV[4]

-- Dedupe check: same request_id -> no-op, return current value.
local seen = redis.call('HGET', applied_key, req_id)
if seen then
	local cur = redis.call('ZSCORE', zset_key, player_id) or '0'
	return { 0, cur, seen }
end

-- Write the profile and the initial score together.
redis.call('HSET', profile_key, 'player_name', player_name)
redis.call('ZADD', zset_key, score, player_id)

-- Append the fact to the ledger and remember the request_id.
local id = redis.call(
	'XADD', stream_key, '*',
	'type', 'set',
	'player_id', player_id,
	'amount', score,
	'request_id', req_id
)

redis.call('HSET', applied_key, req_id, id)

return { 1, tostring(score), id }
