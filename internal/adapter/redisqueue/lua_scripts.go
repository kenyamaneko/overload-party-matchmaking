package redisqueue

// 同一 playerID の既存エントリを削除してから ZADD する（冪等 enqueue）
const enqueueScript = `
local prefix = ARGV[1] .. ":"
local members = redis.call('ZRANGE', KEYS[1], 0, -1)
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    redis.call('ZREM', KEYS[1], m)
  end
end
local newMember = ARGV[1] .. ":" .. ARGV[2]
return redis.call('ZADD', KEYS[1], ARGV[3], newMember)
`

// 2 人以上いる場合のみ先頭 2 件をアトミックに pop する
const popPairScript = `
local count = redis.call('ZCARD', KEYS[1])
if count < 2 then
  return {}
end
return redis.call('ZPOPMIN', KEYS[1], 2)
`

// 指定 playerID のエントリを全て削除する
const cancelScript = `
local prefix = ARGV[1] .. ":"
local members = redis.call('ZRANGE', KEYS[1], 0, -1)
local removed = 0
for _, m in ipairs(members) do
  if string.sub(m, 1, #prefix) == prefix then
    removed = removed + redis.call('ZREM', KEYS[1], m)
  end
end
return removed
`

// 元の score で ZADD し直す（FIFO 順序の保持）
const reenqueueScript = `
local i = 1
while i <= #ARGV do
  local member = ARGV[i] .. ":" .. ARGV[i+1]
  redis.call('ZADD', KEYS[1], ARGV[i+2], member)
  i = i + 3
end
return 1
`
