-- 2 人以上いる場合のみ先頭 2 件をアトミックに pop する
local queueKey = KEYS[1]

local count = redis.call('ZCARD', queueKey)
if count < 2 then
  return {}
end
return redis.call('ZPOPMIN', queueKey, 2)
