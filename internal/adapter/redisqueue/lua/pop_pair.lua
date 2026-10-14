-- 2 人以上いる場合のみ先頭 2 件をアトミックに pop し、取り出した時点の gatewayInstanceID も返す。
-- 戻り値は {instanceID, member1, score1, member2, score2}。2 人未満なら空の配列を返す。
-- Reenqueue も未保存を空文字として扱うため、ここでも未保存なら空文字を返す。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]

local count = redis.call('ZCARD', queueKey)
if count < 2 then
  return {}
end
local pair = redis.call('ZPOPMIN', queueKey, 2)
local result = {redis.call('GET', instanceKey) or ''}
for _, value in ipairs(pair) do
  table.insert(result, value)
end
return result
