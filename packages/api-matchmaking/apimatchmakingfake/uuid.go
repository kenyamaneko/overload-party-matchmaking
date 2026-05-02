package apimatchmakingfake

import (
	"crypto/rand"
	"fmt"
)

// newMatchID は `mch_` プレフィックス付きの 16 bytes ランダム hex 文字列を返す。
// api-matchmaking module の外部依存を増やさないため、ULID ライブラリではなく crypto/rand で自前生成する。
func newMatchID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("apimatchmakingfake: crypto/rand.Read failed: %v", err))
	}
	return "mch_" + fmt.Sprintf("%x", b)
}
