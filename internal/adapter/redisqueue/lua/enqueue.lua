-- gatewayInstanceID が保持値と異なる場合、別プロセスへの切り替わりとみなしキューを空にしてから登録する。
-- Upstash のエビクションで識別子キーだけが失われてもキューの中身は残り得るため、GET が nil (未保存) の場合も異なる値として扱う。
-- member 文字列の組み立ては Go 側で行い、Lua は ZADD/ZREM のみ責務として持つ。
-- 既存エントリの検出は `playerID:` 前置一致で行う (member は playerID 始まり想定)。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]
local playerID    = ARGV[1]
local member      = ARGV[2]
local score       = ARGV[3]
local instanceID  = ARGV[4]

local storedInstanceID = redis.call('GET', instanceKey)
local removedCount = 0
if storedInstanceID ~= instanceID then
  removedCount = redis.call('ZCARD', queueKey)
  redis.call('DEL', queueKey)
end
redis.call('SET', instanceKey, instanceID)

local prefix = playerID .. ":"
local members = redis.call('ZRANGE', queueKey, 0, -1)
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    redis.call('ZREM', queueKey, m)
  end
end
redis.call('ZADD', queueKey, score, member)

return removedCount
