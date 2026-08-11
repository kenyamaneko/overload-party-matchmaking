package matcher_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Queue = (*fakeQueue)(nil)

// fakeQueue は port.Queue のテスト用フェイク。実際に FIFO のエントリ一覧を
// 保持し、PopPair / Reenqueue はその一覧に対して動作する。Enqueue / Cancel は
// matcher パッケージのテストで呼ばれる想定が無いため panic で検出する。
type fakeQueue struct {
	mu         sync.Mutex
	entries    []domain.QueueEntry
	instanceID string

	popPairErr                error
	reenqueueErr              error
	reenqueueRejectInstanceID bool
}

// newFakeQueueWithPairs は playerPairCount 組 (2 * playerPairCount エントリ) を
// 保持する fakeQueue を返す。各エントリの内容はプレイヤー番号から一意に導出する。
func newFakeQueueWithPairs(playerPairCount int) *fakeQueue {
	q := &fakeQueue{instanceID: "gw-instance-1"}
	for i := 0; i < playerPairCount*2; i++ {
		n := i + 1
		q.entries = append(q.entries, domain.QueueEntry{
			PlayerID: fmt.Sprintf("player-%d", n),
			DeckID:   int64(n),
			Name:     fmt.Sprintf("Name%d", n),
			Level:    int64(n),
		})
	}
	return q
}

func (q *fakeQueue) PopPair(context.Context) ([]domain.QueueEntry, string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.popPairErr != nil {
		return nil, "", q.popPairErr
	}
	if len(q.entries) < 2 {
		return nil, "", nil
	}
	pair := append([]domain.QueueEntry(nil), q.entries[:2]...)
	q.entries = q.entries[2:]
	return pair, q.instanceID, nil
}

func (q *fakeQueue) Reenqueue(_ context.Context, entries []domain.QueueEntry, gatewayInstanceID string) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.reenqueueErr != nil {
		return false, q.reenqueueErr
	}
	if q.reenqueueRejectInstanceID || gatewayInstanceID != q.instanceID {
		return false, nil
	}
	// 元の FIFO 順序を保つため、書き戻すエントリを先頭に戻す。
	q.entries = append(append([]domain.QueueEntry(nil), entries...), q.entries...)
	return true, nil
}

func (q *fakeQueue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func (q *fakeQueue) entriesSnapshot() []domain.QueueEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]domain.QueueEntry, len(q.entries))
	copy(out, q.entries)
	return out
}

func (q *fakeQueue) Enqueue(context.Context, string, int64, string, int64, string) (int64, error) {
	panic("fakeQueue: Enqueue is not expected to be called")
}

func (q *fakeQueue) Cancel(context.Context, string) (bool, error) {
	panic("fakeQueue: Cancel is not expected to be called")
}

func (q *fakeQueue) Size(context.Context) (int64, error) {
	panic("fakeQueue: Size is not expected to be called")
}

var _ port.RawEventPublisher = (*fakePublisher)(nil)

type publishedMessage struct {
	eventType string
	payload   []byte
}

// fakePublisher は port.RawEventPublisher のテスト用フェイク。
//
// gate が設定されていれば各 Publish 呼び出しはそのラウンドの結果をテスト側が
// 送るまでブロックする (段階ごとの状態を正確に観測する必要があるケース用)。
// gate が nil のときは err を毎回そのまま返す (単純な成功/失敗固定ケース用)。
type fakePublisher struct {
	mu        sync.Mutex
	published []publishedMessage
	callCount int

	err  error
	gate chan error
}

func (p *fakePublisher) Publish(_ context.Context, eventType string, payload []byte) error {
	var err error
	if p.gate != nil {
		err = <-p.gate
	} else {
		err = p.err
	}

	p.mu.Lock()
	p.callCount++
	if err == nil {
		buf := append([]byte(nil), payload...)
		p.published = append(p.published, publishedMessage{eventType: eventType, payload: buf})
	}
	p.mu.Unlock()
	return err
}

func (p *fakePublisher) publishedMessages() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]publishedMessage, len(p.published))
	copy(out, p.published)
	return out
}

func (p *fakePublisher) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}
