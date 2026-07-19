--[[
	Performs an atomic player profile create-or-conflict:
		- reject if the profile already exists
		- write the profile hash

	KEYS[1] = player profile hash  (player:{player_id})

	ARGV[1] = player_name          (display name)
	ARGV[2] = created_at           (profile timestamp)

	Returns: 1 created, 0 already exists
]]

local profile_key = KEYS[1]
local player_name, created_at = ARGV[1], ARGV[2]

if redis.call('EXISTS', profile_key) == 1 then
	return 0
end

redis.call('HSET', profile_key, 'player_name', player_name, 'created_at', created_at)

return 1
