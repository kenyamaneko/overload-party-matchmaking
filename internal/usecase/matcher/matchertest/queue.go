package matchertest

import (
	"context"
	"sync"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Queue = (*Queue)(nil)

// PopStep は Queue の取り出し 1 回分の結果です。
type PopStep struct {
	isPair bool
	run    func() ([]domain.QueueEntry, string, error)
}

// ReenqueueStep は Queue の書き戻し 1 回分の結果です。
type ReenqueueStep func() (bool, error)

// ReenqueueRequest は Queue が受け取った書き戻しの要求です。
type ReenqueueRequest struct {
	Entries           []domain.QueueEntry
	GatewayInstanceID string
}

// Queue は事前に決めた結果を返し、受け取った引数を記録する port.Queue の偽物です。
// 書き戻しは、呼び出しがシャットダウンで中断されているとき、事前に決めた結果にかかわらず中断のエラーを返します。
type Queue struct {
	port.Queue

	mu                sync.Mutex
	pops              []PopStep
	popCalls          int
	reenqueues        []ReenqueueStep
	reenqueueRequests []ReenqueueRequest
	inFlight          int
}

// NewQueue は何も記録していない Queue を生成します。
func NewQueue() *Queue {
	return &Queue{}
}

// PairPopped は gatewayInstanceID を保持した状態でペアを取り出す結果を返します。
func PairPopped(gatewayInstanceID string, entries ...domain.QueueEntry) PopStep {
	return PopStep{isPair: true, run: func() ([]domain.QueueEntry, string, error) {
		return entries, gatewayInstanceID, nil
	}}
}

// NothingPopped は待機中が 2 人未満で空を取り出す結果を返します。
func NothingPopped() PopStep {
	return PopStep{run: func() ([]domain.QueueEntry, string, error) {
		return nil, "", nil
	}}
}

// PopFails はエラーを返す取り出しの結果を返します。
func PopFails(err error) PopStep {
	return PopStep{run: func() ([]domain.QueueEntry, string, error) {
		return nil, "", err
	}}
}

// PopBlockedUntil は gate が開くまで ctx を見ずに止まり、そのあと空を返す取り出しの結果を返します。
func PopBlockedUntil(gate *Gate) PopStep {
	return PopStep{run: func() ([]domain.QueueEntry, string, error) {
		<-gate.Opened()
		return nil, "", nil
	}}
}

// ReenqueueSucceeds は書き戻しに成功する結果を返します。
func ReenqueueSucceeds() ReenqueueStep {
	return func() (bool, error) { return true, nil }
}

// ReenqueueSkipped は gateway_instance_id が取り出したときと異なり、書き戻さずに終わる結果を返します。
func ReenqueueSkipped() ReenqueueStep {
	return func() (bool, error) { return false, nil }
}

// ReenqueueFails はエラーを返す書き戻しの結果を返します。
func ReenqueueFails(err error) ReenqueueStep {
	return func() (bool, error) { return false, err }
}

// ReenqueueFailsAfterShutdown は呼び出しの中で shutdown を呼んでからエラーを返す書き戻しの結果を返します。
func ReenqueueFailsAfterShutdown(shutdown func(), err error) ReenqueueStep {
	return func() (bool, error) {
		shutdown()
		return false, err
	}
}

// AddPops は取り出しの結果を呼ばれる順に加えます。なくなったあとの取り出しは空を返します。
func (q *Queue) AddPops(steps ...PopStep) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pops = append(q.pops, steps...)
}

// AddReenqueues は書き戻しの結果を呼ばれる順に加えます。なくなったあとの書き戻しは成功します。
func (q *Queue) AddReenqueues(steps ...ReenqueueStep) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reenqueues = append(q.reenqueues, steps...)
}

// PopPair は事前に決めた次の結果を返します。結果がなくなっていれば空を返します。
func (q *Queue) PopPair(_ context.Context) ([]domain.QueueEntry, string, error) {
	q.mu.Lock()
	q.popCalls++
	q.inFlight++
	step := NothingPopped()
	if len(q.pops) > 0 {
		step = q.pops[0]
		q.pops = q.pops[1:]
	}
	q.mu.Unlock()
	defer q.finishCall()
	return step.run()
}

// Reenqueue は要求を記録し、事前に決めた次の結果を返します。呼び出しの ctx が取り消し済みなら ctx のエラーを返します。
func (q *Queue) Reenqueue(ctx context.Context, entries []domain.QueueEntry, gatewayInstanceID string) (bool, error) {
	q.mu.Lock()
	q.inFlight++
	q.reenqueueRequests = append(q.reenqueueRequests, ReenqueueRequest{
		Entries:           append([]domain.QueueEntry(nil), entries...),
		GatewayInstanceID: gatewayInstanceID,
	})
	step := ReenqueueSucceeds()
	if len(q.reenqueues) > 0 {
		step = q.reenqueues[0]
		q.reenqueues = q.reenqueues[1:]
	}
	interruption := ctx.Err()
	q.mu.Unlock()
	defer q.finishCall()
	reenqueued, err := step()
	if interruption != nil {
		return false, interruption
	}
	return reenqueued, err
}

func (q *Queue) finishCall() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.inFlight--
}

// PopCalls は取り出しが呼ばれた回数を返します。
func (q *Queue) PopCalls() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.popCalls
}

// RemainingPairs はまだ取り出しで返していないペアの数を返します。
func (q *Queue) RemainingPairs() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, step := range q.pops {
		if step.isPair {
			n++
		}
	}
	return n
}

// ReenqueueRequests は受け取った書き戻しの要求を呼ばれた順に返します。
func (q *Queue) ReenqueueRequests() []ReenqueueRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]ReenqueueRequest(nil), q.reenqueueRequests...)
}

// CallCount は取り出しと書き戻しが呼ばれた回数の合計を返します。
func (q *Queue) CallCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.popCalls + len(q.reenqueueRequests)
}

// HasCallsInFlight は返っていない呼び出しがあるかを返します。
func (q *Queue) HasCallsInFlight() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.inFlight > 0
}
