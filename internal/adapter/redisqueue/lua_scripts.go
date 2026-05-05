package redisqueue

import _ "embed"

//go:embed lua/enqueue.lua
var enqueueScript string

//go:embed lua/pop_pair.lua
var popPairScript string

//go:embed lua/cancel.lua
var cancelScript string

//go:embed lua/reenqueue.lua
var reenqueueScript string
