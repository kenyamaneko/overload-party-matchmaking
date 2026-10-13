package matchertest

import (
	"context"
	"sync"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

var _ port.RawEventPublisher = (*Publisher)(nil)

// PublishStep は Publisher の発行 1 回分の結果です。
type PublishStep func(ctx context.Context) error

// Publication は Publisher が記録した発行です。
type Publication struct {
	EventType string
	Payload   []byte
}

// Publisher は事前に決めた結果を返し、発行の内容を記録する port.RawEventPublisher の偽物です。
type Publisher struct {
	mu           sync.Mutex
	steps        []PublishStep
	publications []Publication
	inFlight     int
}

// NewPublisher は何も記録していない Publisher を生成します。
func NewPublisher() *Publisher {
	return &Publisher{}
}

// PublishSucceeds は発行に成功する結果を返します。
func PublishSucceeds() PublishStep {
	return func(context.Context) error { return nil }
}

// PublishFails はエラーを返す発行の結果を返します。
func PublishFails(err error) PublishStep {
	return func(context.Context) error { return err }
}

// PublishBlockedUntil は gate が開くまで ctx を見ずに止まり、そのあと成功する発行の結果を返します。
func PublishBlockedUntil(gate *Gate) PublishStep {
	return func(context.Context) error {
		<-gate.Opened()
		return nil
	}
}

// PublishInterruptedByShutdown は ctx が取り消されるまで止まり、取り消されると ctx のエラーを返す発行の結果を返します。
func PublishInterruptedByShutdown() PublishStep {
	return func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
}

// AddSteps は発行の結果を呼ばれる順に加えます。なくなったあとの発行は成功します。
func (p *Publisher) AddSteps(steps ...PublishStep) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.steps = append(p.steps, steps...)
}

// Publish は発行を記録し、事前に決めた次の結果を返します。
func (p *Publisher) Publish(ctx context.Context, eventType string, payload []byte) error {
	p.mu.Lock()
	p.inFlight++
	p.publications = append(p.publications, Publication{EventType: eventType, Payload: append([]byte(nil), payload...)})
	step := PublishSucceeds()
	if len(p.steps) > 0 {
		step = p.steps[0]
		p.steps = p.steps[1:]
	}
	p.mu.Unlock()
	defer p.finishCall()
	return step(ctx)
}

func (p *Publisher) finishCall() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inFlight--
}

// Publications は記録した発行を呼ばれた順に返します。
func (p *Publisher) Publications() []Publication {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Publication(nil), p.publications...)
}

// HasCallsInFlight は返っていない発行があるかを返します。
func (p *Publisher) HasCallsInFlight() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inFlight > 0
}
