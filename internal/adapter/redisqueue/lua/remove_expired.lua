-- score (joinedAt ミリ秒) が cutoff 以下のエントリを削除する
local queueKey = KEYS[1]
local cutoff   = ARGV[1]

return redis.call('ZREMRANGEBYSCORE', queueKey, '-inf', cutoff)
