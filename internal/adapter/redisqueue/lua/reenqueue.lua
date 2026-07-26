-- ARGV[1] の gatewayInstanceID が保持値と一致する場合のみ、元の score で ZADD し直す (FIFO 順序の保持)。
-- ARGV[2] 以降は (member, score) の 2 つ組を可変長で受け取る。
-- member 文字列の組み立ては Go 側で行う (Lua から encoding 知識を排除する設計)。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]
local expectedID  = ARGV[1]

-- 登録のたびに識別子キーへ書き込むため、pop 時点・書き戻し時点のどちらも未保存なら間に登録が無く
-- gateway プロセスは切り替わっていないとみなし、GET の未保存 (false) を空文字に正規化してから比較する。
local stored = redis.call('GET', instanceKey)
if stored == false then
  stored = ''
end
if stored ~= expectedID then
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
