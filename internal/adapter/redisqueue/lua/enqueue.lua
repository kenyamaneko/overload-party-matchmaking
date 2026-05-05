-- 同一 playerID の既存エントリを削除してから ZADD する（冪等 enqueue）
local queueKey = KEYS[1]
local playerID = ARGV[1]
local deckID   = ARGV[2]
local score    = ARGV[3]

local prefix = playerID .. ":"
local members = redis.call('ZRANGE', queueKey, 0, -1)
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    redis.call('ZREM', queueKey, m)
  end
end
local newMember = playerID .. ":" .. deckID
return redis.call('ZADD', queueKey, score, newMember)
