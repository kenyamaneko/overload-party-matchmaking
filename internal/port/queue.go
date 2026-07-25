package port

import (
	"context"
	"time"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Queue はマッチメイキングキュー操作を抽象化します。
// Enqueue は player summary (name / level) を含めて queue entry に保存する。
// JoinedAt は実装側で設定する (Reenqueue は元の JoinedAt を保持するため entries で受ける)。
// RemoveExpired が対象とする閾値は呼び出し側が決める (Queue 自身は期限切れの定義を持たない)。
type Queue interface {
	Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64) error
	Cancel(ctx context.Context, playerID string) (bool, error)
	Size(ctx context.Context) (int64, error)
	PopPair(ctx context.Context) ([]domain.QueueEntry, error)
	Reenqueue(ctx context.Context, entries []domain.QueueEntry) error
	RemoveExpired(ctx context.Context, before time.Time) (int64, error)
}
