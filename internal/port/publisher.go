package port

import (
	"context"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

// Publisher はマッチ成立イベントの発行を抽象化します。
type Publisher interface {
	PublishMatchMade(ctx context.Context, event domain.MatchMadeEvent) error
	Close() error
}
