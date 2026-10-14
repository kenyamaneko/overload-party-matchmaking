-- 各スクリプトの先頭に連結して使う共通関数。
-- member の先頭フィールド (最初の `:` の手前) が playerID と完全に一致するエントリを全て削除し、削除件数を返す。
-- 前方一致で探すと `:` を含む playerID が別のプレイヤーのエントリに一致するため、先頭フィールドで比較する。
local function removePlayerEntries(queueKey, playerID)
  local removed = 0
  for _, member in ipairs(redis.call('ZRANGE', queueKey, 0, -1)) do
    if string.match(member, '^[^:]*') == playerID then
      removed = removed + redis.call('ZREM', queueKey, member)
    end
  end
  return removed
end
