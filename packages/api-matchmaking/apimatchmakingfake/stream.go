package apimatchmakingfake

import (
	"context"
	"testing"
	"time"
)

// Stream は特定 topic に subscribe 済みの observable stream。
type Stream struct {
	ch      <-chan []byte
	topic   string
	handled chan error
}

// NewStream は Subscriber から指定 topic のメッセージを読む Stream を返す。
// subscribe を eager に行うことで、Consume を goroutine で起動する前後の publish が
// 失われない。
func NewStream(subscriber *Subscriber, topic string) *Stream {
	return &Stream{
		ch: subscriber.Messages(topic),
		// topic は ExpectHandled が timeout 時に診断ログを出すためだけに保持。
		topic:   topic,
		handled: make(chan error, handledBufferSize),
	}
}

// handledBufferSize は handled channel の buffer サイズ。
// 非ゼロにすることで Consume が ExpectHandled の回収を待たずに進める。
const handledBufferSize = 16

// Consume は subscribe 済み topic のメッセージを handler に渡し続け、戻り値を handled に流す。
// ctx キャンセル時 / channel クローズ時は nil で正常終了する。
func (s *Stream) Consume(ctx context.Context, handler func(ctx context.Context, data []byte) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case data, ok := <-s.ch:
			if !ok {
				return nil
			}
			s.handled <- handler(ctx, data)
		}
	}
}

// ExpectHandled は 1 メッセージ分の handler 戻り値を timeout 付きで取り出す。
// timeout 超過時は t.Fatal で即失敗する。
func (s *Stream) ExpectHandled(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-s.handled:
		return err
	case <-time.After(timeout):
		t.Fatalf("apimatchmakingfake.Stream[%s]: timeout waiting for handler completion (%s)", s.topic, timeout)
		return nil
	}
}
