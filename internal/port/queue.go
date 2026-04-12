package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Queue はマッチメイキングキュー操作を抽象化します。
type Queue interface {
	Enqueue(ctx context.Context, playerID string, deckID int64) error
	Cancel(ctx context.Context, playerID string) (bool, error)
	Size(ctx context.Context) (int64, error)
	PopPair(ctx context.Context) ([]domain.QueueEntry, error)
	Reenqueue(ctx context.Context, entries []domain.QueueEntry) error
}
