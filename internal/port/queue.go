package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Queue はマッチメイキングキュー操作を抽象化します。
// Enqueue は player summary (name / level) を含めて queue entry に保存する。
// JoinedAt は実装側で設定する (Reenqueue は元の JoinedAt を保持するため entries で受ける)。
// Enqueue は gatewayInstanceID が直前と異なるときキューを空にしてから登録し、削除件数を返す。
type Queue interface {
	Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) (int64, error)
	Cancel(ctx context.Context, playerID string) (bool, error)
	Size(ctx context.Context) (int64, error)
	PopPair(ctx context.Context) ([]domain.QueueEntry, error)
	Reenqueue(ctx context.Context, entries []domain.QueueEntry) error
}
