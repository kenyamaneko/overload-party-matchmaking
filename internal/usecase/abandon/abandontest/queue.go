// Package abandontest は abandon のテストがキューを扱うための偽物を提供する。
package abandontest

import (
	"context"
	"sync"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.Queue = (*Queue)(nil)

// CancelStep は Queue の取り消し 1 回分の結果です。
type CancelStep func() (bool, error)

// Queue は事前に決めた結果を返し、取り消しを求められたプレイヤー識別子を記録する port.Queue の偽物です。
type Queue struct {
	port.Queue

	mu       sync.Mutex
	steps    map[string][]CancelStep
	requests []string
}

// NewQueue は何も記録していない Queue を生成します。
func NewQueue() *Queue {
	return &Queue{steps: map[string][]CancelStep{}}
}

// PlayerWaiting は待機にいたプレイヤーの取り消しに成功する結果を返します。
func PlayerWaiting() CancelStep {
	return func() (bool, error) { return true, nil }
}

// PlayerNotWaiting は待機にいなかったプレイヤーの取り消しがエラーにならない結果を返します。
func PlayerNotWaiting() CancelStep {
	return func() (bool, error) { return false, nil }
}

// CancelFails はエラーを返す取り消しの結果を返します。
func CancelFails(err error) CancelStep {
	return func() (bool, error) { return false, err }
}

// AddCancels は playerID の取り消しの結果を呼ばれる順に加えます。なくなったあとの取り消しは待機にいなかった結果を返します。
func (q *Queue) AddCancels(playerID string, steps ...CancelStep) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.steps[playerID] = append(q.steps[playerID], steps...)
}

// Cancel は playerID を記録し、事前に決めた次の結果を返します。
func (q *Queue) Cancel(_ context.Context, playerID string) (bool, error) {
	q.mu.Lock()
	q.requests = append(q.requests, playerID)
	step := PlayerNotWaiting()
	if len(q.steps[playerID]) > 0 {
		step = q.steps[playerID][0]
		q.steps[playerID] = q.steps[playerID][1:]
	}
	q.mu.Unlock()
	return step()
}

// CancelRequests は取り消しを求められたプレイヤー識別子を呼ばれた順に返します。
func (q *Queue) CancelRequests() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.requests...)
}
