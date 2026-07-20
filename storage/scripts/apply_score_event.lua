--[[
	Performs an atomic score write:
		- (if idempotency key) idempotency check (replay a matching write, reject a conflicting one)
		- Existence check (player, board)
		- apply to the board's projection
		- append event
		- (if idempotency key) record the idempotency key

	KEYS[1] = projection zset      (app:view:leaderboard:{board_id})
	KEYS[2] = event stream         (app:ledger:events)
	KEYS[3] = idempotency hash     (app:ledger:idempotency)
	KEYS[4] = player profile hash  (app:player:profile:{player_id})
	KEYS[5] = board hash           (app:board:profile:{board_id})

	ARGV[1] = operation type       ("set" | "increment")
	ARGV[2] = player_id           
	ARGV[3] = amount               (float, raw string)
	ARGV[4] = request_id           (server-generated UUID)
	ARGV[5] = board_id             
	ARGV[6] = idempotency_key      (optional)

	Returns: { code, entry_id }
		code:  1 applied | 0 deduped (same key, same fingerprint -> same entry_id)
		      -1 player not found | -2 board not found | -3 board closed
		      -4 idempotency key fingerptint mismatch
]]

local zset_key, stream_key, idempotency_key, profile_key, board_key =
	KEYS[1], KEYS[2], KEYS[3], KEYS[4], KEYS[5]
local op_type, player_id, amount, req_id, board_id, idem_key =
	ARGV[1], ARGV[2], ARGV[3], ARGV[4], ARGV[5], ARGV[6]

-- Idempotency check
-- Record value is "entry_id|op|amount"
local dedupe_field
local have_key = idem_key ~= ''
if have_key then
	dedupe_field = board_id .. ':' .. player_id .. ':' .. idem_key
	local seen = redis.call('HGET', idempotency_key, dedupe_field)
	if seen then
		local sep1 = string.find(seen, '|', 1, true)
		local sep2 = string.find(seen, '|', sep1 + 1, true)
		local seen_entry = string.sub(seen, 1, sep1 - 1)
		local seen_op = string.sub(seen, sep1 + 1, sep2 - 1)
		local seen_amount = string.sub(seen, sep2 + 1)
		if seen_op == op_type and seen_amount == amount then
			return { 0, seen_entry }
		end
		return { -4, '' }
	end
end

-- Existence check
if redis.call('EXISTS', profile_key) == 0 then
	return { -1, '' }
end
if redis.call('EXISTS', board_key) == 0 then
	return { -2, '' }
end
if redis.call('HGET', board_key, 'board_state') == 'closed' then
	return { -3, '' }
end

-- Apply to the projection
if op_type == 'set' then
	redis.call('ZADD', zset_key, amount, player_id)
else
	redis.call('ZINCRBY', zset_key, amount, player_id)
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

-- Record the idempotency
if have_key then
	redis.call('HSET', idempotency_key, dedupe_field, entry_id .. '|' .. op_type .. '|' .. amount)
	redis.call('HEXPIRE', idempotency_key, 86400, 'FIELDS', 1, dedupe_field)
end

return { 1, entry_id }
