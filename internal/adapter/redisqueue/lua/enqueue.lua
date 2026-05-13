-- 同一 playerID の既存エントリを削除してから ZADD する（冪等 enqueue）
-- member 文字列の組み立ては Go 側で行い、Lua は ZADD/ZREM のみ責務として持つ。
-- 既存エントリの検出は `playerID:` 前置一致で行う (member は playerID 始まり想定)。
local queueKey = KEYS[1]
local playerID = ARGV[1]
local member   = ARGV[2]
local score    = ARGV[3]

local prefix = playerID .. ":"
local members = redis.call('ZRANGE', queueKey, 0, -1)
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    redis.call('ZREM', queueKey, m)
  end
end
return redis.call('ZADD', queueKey, score, member)
