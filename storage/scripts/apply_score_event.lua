--[[
	Performs an atomic score write:
		- idempotency check (same client req against same board hits same record)
		- Existence check (player, board)
		- apply to the board's projection
		- append event
		- record the dedupe key

	KEYS[1] = projection zset      (leaderboard:{board_id})
	KEYS[2] = event stream         (ledger:events)
	KEYS[3] = idempotency hash     (ledger:idempotency)
	KEYS[4] = player profile hash  (player:{player_id})
	KEYS[5] = board hash           (board:{board_id})

	ARGV[1] = operation type       ("set" | "increment")
	ARGV[2] = player_id            (UUID)
	ARGV[3] = amount               (float)
	ARGV[4] = request_id           (UUID)
	ARGV[5] = board_id             (client-chosen id)

	Returns: { code, new_score (string), stream_id }
		code:  1 applied | 0 deduped (original result)
		      -1 player not found | -2 board not found
		      (-3 reserved for "board closed")

	A rejected write (negative code) appends nothing: events record facts only (rule 2).
	Result codes must match the applyCode* constants in storage.go.
]]

local zset_key, stream_key, idempotency_key, profile_key, board_key =
	KEYS[1], KEYS[2], KEYS[3], KEYS[4], KEYS[5]
local op_type, player_id, amount, req_id, board_id =
	ARGV[1], ARGV[2], ARGV[3], ARGV[4], ARGV[5]

-- Idempotency check
local dedupe_key = board_id .. ':' .. player_id .. ':' .. req_id
local seen = redis.call('HGET', idempotency_key, dedupe_key)
if seen then
	local cur = redis.call('ZSCORE', zset_key, player_id) or '0'
	return { 0, cur, seen }
end

-- Existence check
if redis.call('EXISTS', profile_key) == 0 then
	return { -1, '', '' }
end
if redis.call('EXISTS', board_key) == 0 then
	return { -2, '', '' }
end

-- Apply to the projection
local newscore
if op_type == 'set' then
	redis.call('ZADD', zset_key, amount, player_id)
	newscore = amount
else
	newscore = redis.call('ZINCRBY', zset_key, amount, player_id)
end

-- Append the fact to the ledger
local entry_id = redis.call(
	'XADD', stream_key, '*',
	'type', op_type,
	'player_id', player_id,
	'board_id', board_id,
	'amount', amount,
	'request_id', req_id
)

redis.call('HSET', idempotency_key, dedupe_key, entry_id)

return { 1, tostring(newscore), entry_id }
