// Package httphandlertest は httphandler のテストが共有する偽物と HTTP 要求の補助を提供する。
package httphandlertest

import (
	"context"
	"errors"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

var errUnsupported = errors.New("TST-unsupported-operation")

// FakeQueue は追加・除外・サイズ取得の戻り値を指定できる port.Queue の偽物。
type FakeQueue struct {
	EnqueueRemovedCount int64
	EnqueueErr          error
	CancelRemoved       bool
	CancelErr           error
	QueueSize           int64
	SizeErr             error
}

// NewFailingQueue は追加・除外・サイズ取得のすべてが message のエラーを返す FakeQueue を返す。
func NewFailingQueue(message string) *FakeQueue {
	err := errors.New(message)
	return &FakeQueue{EnqueueErr: err, CancelErr: err, SizeErr: err}
}

// Enqueue は EnqueueRemovedCount と EnqueueErr を返す。
func (q *FakeQueue) Enqueue(_ context.Context, _ string, _ int64, _ string, _ int64, _ string) (int64, error) {
	return q.EnqueueRemovedCount, q.EnqueueErr
}

// Cancel は CancelRemoved と CancelErr を返す。
func (q *FakeQueue) Cancel(_ context.Context, _ string) (bool, error) {
	return q.CancelRemoved, q.CancelErr
}

// Size は QueueSize と SizeErr を返す。
func (q *FakeQueue) Size(_ context.Context) (int64, error) {
	return q.QueueSize, q.SizeErr
}

// PopPair は常にエラーを返す。ハンドラは呼ばない。
func (q *FakeQueue) PopPair(_ context.Context) ([]domain.QueueEntry, string, error) {
	return nil, "", errUnsupported
}

// Reenqueue は常にエラーを返す。ハンドラは呼ばない。
func (q *FakeQueue) Reenqueue(_ context.Context, _ []domain.QueueEntry, _ string) (bool, error) {
	return false, errUnsupported
}

// FakeAbandoner は Err を返す httphandler.MatchAbandoner の偽物。
type FakeAbandoner struct {
	Err error
}

// Abandon は Err を返す。
func (a *FakeAbandoner) Abandon(_ context.Context, _ string, _ []string, _ string) error {
	return a.Err
}

// FakeCircuit は Open を返す httphandler.CircuitStater の偽物。
type FakeCircuit struct {
	Open bool
}

// IsCircuitOpen は Open を返す。
func (c *FakeCircuit) IsCircuitOpen() bool {
	return c.Open
}
