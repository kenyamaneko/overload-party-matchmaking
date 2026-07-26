-- Reenqueue は pop 済みペアを publish 失敗時に書き戻す経路であり、pop 時点と異なる gateway
-- instance に切り替わっていたら書き戻さない (ZPOPMIN で取り出す 1 ペアは常に同じ gateway
-- 由来なので、ペア全体に対して識別子 1 つで判定できる)。
-- 一致する場合のみ元の score で ZADD し直す (FIFO 順序の保持)。
-- ARGV[1] は pop 時点の gatewayInstanceID、以降は (member, score) の 2 つ組を可変長で受け取る。
-- member 文字列の組み立ては Go 側で行う (Lua から encoding 知識を排除する設計)。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]
local expectedID  = ARGV[1]

if redis.call('GET', instanceKey) ~= expectedID then
  return 0
end

local written = 0
local i = 2
while i <= #ARGV do
  local member = ARGV[i]
  local score  = ARGV[i+1]
  redis.call('ZADD', queueKey, score, member)
  written = written + 1
  i = i + 2
end
return written
