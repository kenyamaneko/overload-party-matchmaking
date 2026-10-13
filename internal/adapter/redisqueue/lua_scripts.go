package redisqueue

import _ "embed"

//go:embed lua/remove_player_entries.lua
var removePlayerEntriesFunction string

//go:embed lua/enqueue.lua
var enqueueBody string

//go:embed lua/pop_pair.lua
var popPairScript string

//go:embed lua/cancel.lua
var cancelBody string

//go:embed lua/reenqueue.lua
var reenqueueBody string

// 同一 playerID のエントリを取り除く処理を enqueue・cancel・reenqueue で共有するため、共通関数を先頭に連結する。
var (
	enqueueScript   = removePlayerEntriesFunction + "\n" + enqueueBody
	cancelScript    = removePlayerEntriesFunction + "\n" + cancelBody
	reenqueueScript = removePlayerEntriesFunction + "\n" + reenqueueBody
)
