-- 2 人以上いる場合のみ先頭 2 件をアトミックに pop する。取り出した時点で保持していた
-- gatewayInstanceID も合わせて返し、Reenqueue が同じ gateway 由来かどうかを判定できるように
-- する。Reenqueue も未保存を空文字として比較するため、ここでも未保存なら空文字を返す。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]

local count = redis.call('ZCARD', queueKey)
if count < 2 then
  return {}
end
local pair = redis.call('ZPOPMIN', queueKey, 2)
local instanceID = redis.call('GET', instanceKey) or ''
return {instanceID, pair}
