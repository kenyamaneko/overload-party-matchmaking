package abandon_test

import (
	"context"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Queue = (*fakeQueue)(nil)

// fakeQueue は port.Queue のテスト用フェイク。Abandoner が使う Cancel のみ
// 実データを保持し、他メソッドは呼ばれる想定が無いため panic で検出する。
type fakeQueue struct {
	entries             map[string]bool
	cancelErrByPlayerID map[string]error
}

func newFakeQueue(playerIDs ...string) *fakeQueue {
	entries := make(map[string]bool)
	for _, id := range playerIDs {
		entries[id] = true
	}
	return &fakeQueue{
		entries:             entries,
		cancelErrByPlayerID: make(map[string]error),
	}
}

func (q *fakeQueue) Cancel(_ context.Context, playerID string) (bool, error) {
	if err, ok := q.cancelErrByPlayerID[playerID]; ok {
		return false, err
	}
	if !q.entries[playerID] {
		return false, nil
	}
	delete(q.entries, playerID)
	return true, nil
}

func (q *fakeQueue) contains(playerID string) bool {
	return q.entries[playerID]
}

func (q *fakeQueue) Enqueue(context.Context, string, int64, string, int64, string) (int64, error) {
	panic("fakeQueue: Enqueue is not expected to be called")
}

func (q *fakeQueue) Size(context.Context) (int64, error) {
	panic("fakeQueue: Size is not expected to be called")
}

func (q *fakeQueue) PopPair(context.Context) ([]domain.QueueEntry, string, error) {
	panic("fakeQueue: PopPair is not expected to be called")
}

func (q *fakeQueue) Reenqueue(context.Context, []domain.QueueEntry, string) (bool, error) {
	panic("fakeQueue: Reenqueue is not expected to be called")
}
