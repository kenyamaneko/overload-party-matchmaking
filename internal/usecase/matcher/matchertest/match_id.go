package matchertest

import "github.com/kenyamaneko/overload-party-matchmaking/internal/port"

var _ port.MatchIDGenerator = FixedMatchIDGenerator{}

// FixedMatchIDGenerator は常に同じマッチ識別子を発行する port.MatchIDGenerator の偽物です。
type FixedMatchIDGenerator struct {
	id string
}

// NewFixedMatchIDGenerator は id を発行する FixedMatchIDGenerator を生成します。
func NewFixedMatchIDGenerator(id string) FixedMatchIDGenerator {
	return FixedMatchIDGenerator{id: id}
}

// NewMatchID は生成時に渡された id を返します。
func (g FixedMatchIDGenerator) NewMatchID() string {
	return g.id
}
