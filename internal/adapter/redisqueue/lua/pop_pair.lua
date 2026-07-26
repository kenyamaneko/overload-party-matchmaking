-- 2 人以上いる場合のみ先頭 2 件をアトミックに pop し、取り出した時点の gatewayInstanceID も返す。
-- Reenqueue も未保存を空文字として扱うため、ここでも未保存なら空文字を返す。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]

local count = redis.call('ZCARD', queueKey)
if count < 2 then
  return {}
end
local pair = redis.call('ZPOPMIN', queueKey, 2)
local instanceID = redis.call('GET', instanceKey) or ''
return {instanceID, pair}
