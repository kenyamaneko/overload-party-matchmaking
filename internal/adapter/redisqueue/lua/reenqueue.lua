-- 元の score で ZADD し直す（FIFO 順序の保持）
-- ARGV は (member, score) の 2 つ組を可変長で受け取る。
-- member 文字列の組み立ては Go 側で行う (Lua から encoding 知識を排除する設計)。
local queueKey = KEYS[1]

local i = 1
while i <= #ARGV do
  local member = ARGV[i]
  local score  = ARGV[i+1]
  redis.call('ZADD', queueKey, score, member)
  i = i + 2
end
return 1
