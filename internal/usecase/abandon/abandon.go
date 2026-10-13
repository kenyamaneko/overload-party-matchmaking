package abandon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

// Abandoner は不成立が申告されたマッチの破棄を担います。
type Abandoner struct {
	queue port.Queue
}

// New は Abandoner を生成します。
func New(queue port.Queue) *Abandoner {
	return &Abandoner{queue: queue}
}

// Abandon は申告されたペアを待機から取り除き、破棄した理由と人数を記録します。
// 取り消しに失敗したプレイヤーがいても残りの取り消しを続け、失敗をまとめて返します。
func (a *Abandoner) Abandon(ctx context.Context, matchID string, playerIDs []string, reason string) error {
	// 配信に失敗したペアは待機に書き戻されるため、成立済みのペアにも取り消しを実行する。
	var errs []error
	for _, id := range playerIDs {
		if _, err := a.queue.Cancel(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("abandon match %s: cancel %s: %w", matchID, id, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	slog.Info("match abandoned", "match_id", matchID, "reason", reason, "player_count", len(playerIDs))
	return nil
}
