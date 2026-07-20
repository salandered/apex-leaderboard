--[[
	Performs an atomic board create-or-conflict:
		- reject if the board already exists
		- write the board hash and register the board id

	KEYS[1] = board hash key    (board:{board_id})
	KEYS[2] = registry zset     (boards)

	ARGV[1] = board_id          (the registry member)
	ARGV[2] = board_name        (mutable display name)
	ARGV[3] = created_at        (board timestamp)
	ARGV[4] = created_at_unix   (registry score: stable creation order for listing)
	ARGV[5] = board_state       ("active" | "closed")

	Returns: 1 created, 0 already exists
]]

local board_key, registry_key = KEYS[1], KEYS[2]
local board_id, board_name, created_at, created_at_unix, board_state =
	ARGV[1], ARGV[2], ARGV[3], ARGV[4], ARGV[5]

if redis.call('EXISTS', board_key) == 1 then
	return 0
end

redis.call('HSET', board_key,
	'board_name', board_name, 'created_at', created_at, 'board_state', board_state)
redis.call('ZADD', registry_key, created_at_unix, board_id)

return 1
