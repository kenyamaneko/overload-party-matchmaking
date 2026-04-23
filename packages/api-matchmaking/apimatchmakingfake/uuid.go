package apimatchmakingfake

import (
	"crypto/rand"
	"fmt"
)

// newMatchID は `mch_` プレフィックス付きの 16 bytes ランダム hex 文字列を返す。
// 本物の matchmaking が発行する ULID (`mch_<ULID>`) 形式と互換を保ち、
// テストが matchID dedup を検証できるよう衝突しにくい値を生成する。
// 本パッケージは api-matchmaking module に属し外部依存を増やさない方針なので
// crypto/rand で自前生成する (ULID ライブラリを入れない)。
func newMatchID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("apimatchmakingfake: crypto/rand.Read failed: %v", err))
	}
	return "mch_" + fmt.Sprintf("%x", b)
}
