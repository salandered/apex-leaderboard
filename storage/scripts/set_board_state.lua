--[[
	Sets the board lifecycle state, only if the board exists:
	an unknown board must not materialize as a state-only hash.

	KEYS[1] = board hash key  (board:{board_id})

	ARGV[1] = board_state     ("active" | "closed")

	Returns: 1 updated, 0 board not found
]]

local board_key = KEYS[1]
local state = ARGV[1]

if redis.call('EXISTS', board_key) == 0 then
	return 0
end

redis.call('HSET', board_key, 'board_state', state)

return 1
