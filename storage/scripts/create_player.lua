--[[
	Performs an atomic player profile create-or-conflict, with optional client idempotency:
		- (if idempotency key) If seen before: replay the original id (same fingerptint)
		  or reject (different fingerptint)
		- reject if the candidate profile already exists
		- write the profile hash
		- (if idempotency key) record the idempotency key with a TTL

	KEYS[1] = player profile hash     (app:player:profile:{candidate_id})
	KEYS[2] = player idempotency hash (app:player:idempotency)

	ARGV[1] = player_name
	ARGV[2] = created_at
	ARGV[3] = candidate player_id    (server-generated id for this attempt)
	ARGV[4] = idempotency_key        (optional)

	Returns: { code, player_id }
		code:  1 created (player_id = candidate)
		       0 deduped (player_id = stored original)
		      -1 candidate id already exists
		      -4 idempotency key fingerprint mismatch
]]

local profile_key, idemp_hash = KEYS[1], KEYS[2]
local player_name, created_at, player_id, idemp_key = ARGV[1], ARGV[2], ARGV[3], ARGV[4]

-- Idempotency check
-- Record value is "player_id|player_name"
local have_key = idemp_key ~= ''
if have_key then
	local seen = redis.call('HGET', idemp_hash, idemp_key)
	if seen then
		local sep = string.find(seen, '|', 1, true)
		local seen_id = string.sub(seen, 1, sep - 1)
		local seen_name = string.sub(seen, sep + 1)
		if seen_name == player_name then
			return { 0, seen_id }
		end
		return { -4, '' }
	end
end

-- Create-or-conflict on the candidate id
if redis.call('EXISTS', profile_key) == 1 then
	return { -1, '' }
end
redis.call('HSET', profile_key, 'player_name', player_name, 'created_at', created_at)

-- Record the idempotency key
if have_key then
	redis.call('HSET', idemp_hash, idemp_key, player_id .. '|' .. player_name)
	redis.call('HEXPIRE', idemp_hash, 86400, 'FIELDS', 1, idemp_key)
end

return { 1, player_id }
