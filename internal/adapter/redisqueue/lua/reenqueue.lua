-- Reenqueue は pop 済みペアを publish 失敗時に書き戻す経路であり、pop 時点と異なる gateway
-- instance に切り替わっていたら書き戻さない (ZPOPMIN で取り出す 1 ペアは常に同じ gateway
-- 由来なので、ペア全体に対して識別子 1 つで判定できる)。
-- 一致する場合のみ元の score で ZADD し直す (FIFO 順序の保持)。
-- ARGV[1] は pop 時点の gatewayInstanceID、以降は (member, score) の 2 つ組を可変長で受け取る。
-- member 文字列の組み立ては Go 側で行う (Lua から encoding 知識を排除する設計)。
local queueKey    = KEYS[1]
local instanceKey = KEYS[2]
local expectedID  = ARGV[1]

-- pop 時点・書き戻し時点のどちらも識別子キーが未保存なら、その間に登録 (必ず識別子
-- キーを書く) が一度も来ておらず gateway instance は切り替わっていないため、GET の
-- 未保存 (false) を空文字に正規化してから比較する。
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
