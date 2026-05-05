-- 指定 playerID のエントリを全て削除する
local queueKey = KEYS[1]
local playerID = ARGV[1]

local prefix = playerID .. ":"
local members = redis.call('ZRANGE', queueKey, 0, -1)
local removed = 0
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    removed = removed + redis.call('ZREM', queueKey, m)
  end
end
return removed
