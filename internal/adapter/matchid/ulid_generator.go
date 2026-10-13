// Package matchid は port.MatchIDGenerator の ULID による実装を提供する。
package matchid

import (
	"crypto/rand"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/port"
)

const matchIDPrefix = "mch_"

var _ port.MatchIDGenerator = (*ULIDGenerator)(nil)

// ULIDGenerator は発行時刻を ULID の時刻部分に持つ `mch_` 始まりのマッチ識別子を発行します。
type ULIDGenerator struct {
	clock port.Clock
}

// NewULIDGenerator は ULIDGenerator を生成します。
func NewULIDGenerator(clock port.Clock) *ULIDGenerator {
	return &ULIDGenerator{clock: clock}
}

// NewMatchID はマッチ識別子を発行します。
func (g *ULIDGenerator) NewMatchID() string {
	id := ulid.MustNew(ulid.Timestamp(g.clock.Now()), rand.Reader)
	return fmt.Sprintf("%s%s", matchIDPrefix, id.String())
}
