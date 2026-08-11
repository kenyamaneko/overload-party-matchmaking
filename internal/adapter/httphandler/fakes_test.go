package httphandler_test

import (
	"context"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Queue = (*fakeQueue)(nil)

// queueEntry は fakeQueue が保持する 1 エントリの登録内容。
type queueEntry struct {
	deckID            int64
	name              string
	level             int64
	gatewayInstanceID string
}

// fakeQueue は port.Queue のテスト用フェイク。handler パッケージのテストでは
// PopPair / Reenqueue は呼ばれる想定が無いため panic で検出する。
type fakeQueue struct {
	entries map[string]queueEntry

	enqueueErr error
	cancelErr  error
	sizeVal    int64
	sizeErr    error
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{entries: make(map[string]queueEntry)}
}

func (q *fakeQueue) Enqueue(_ context.Context, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) (int64, error) {
	if q.enqueueErr != nil {
		return 0, q.enqueueErr
	}
	q.entries[playerID] = queueEntry{deckID: deckID, name: name, level: level, gatewayInstanceID: gatewayInstanceID}
	return 0, nil
}

func (q *fakeQueue) Cancel(_ context.Context, playerID string) (bool, error) {
	if q.cancelErr != nil {
		return false, q.cancelErr
	}
	if _, ok := q.entries[playerID]; !ok {
		return false, nil
	}
	delete(q.entries, playerID)
	return true, nil
}

func (q *fakeQueue) Size(context.Context) (int64, error) {
	if q.sizeErr != nil {
		return 0, q.sizeErr
	}
	return q.sizeVal, nil
}

func (q *fakeQueue) contains(playerID string) bool {
	_, ok := q.entries[playerID]
	return ok
}

func (q *fakeQueue) PopPair(context.Context) ([]domain.QueueEntry, string, error) {
	panic("fakeQueue: PopPair is not expected to be called")
}

func (q *fakeQueue) Reenqueue(context.Context, []domain.QueueEntry, string) (bool, error) {
	panic("fakeQueue: Reenqueue is not expected to be called")
}

// fakeCircuit は httphandler.CircuitStater のテスト用フェイク。
type fakeCircuit struct {
	isOpen bool
}

func (c *fakeCircuit) IsCircuitOpen() bool { return c.isOpen }
