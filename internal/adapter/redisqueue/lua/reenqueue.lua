-- 元の score で ZADD し直す（FIFO 順序の保持）
-- ARGV は (playerID, deckID, score) の 3 つ組を可変長で受け取る
local queueKey = KEYS[1]

local i = 1
while i <= #ARGV do
  local playerID = ARGV[i]
  local deckID   = ARGV[i+1]
  local score    = ARGV[i+2]
  local member   = playerID .. ":" .. deckID
  redis.call('ZADD', queueKey, score, member)
  i = i + 3
end
return 1
