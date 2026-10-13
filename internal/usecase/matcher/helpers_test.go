package matcher_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apimatchmaking "github.com/kenyamaneko/overload-party-matchmaking/packages/api-matchmaking"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/matchid"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/matcher/matchertest"
	"github.com/kenyamaneko/overload-party-matchmaking/internal/usecase/usecasetest"
)

var scenarioStartedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var circuitStates = map[bool]string{true: "open", false: "closed"}

var (
	playerA = domain.QueueEntry{PlayerID: "player-a", DeckID: 101, Name: "alice", Level: 7, JoinedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	playerB = domain.QueueEntry{PlayerID: "player-b", DeckID: 202, Name: "bob", Level: 12, JoinedAt: time.Date(2026, 1, 1, 0, 0, 5, 0, time.UTC)}
)

func playerEntry(playerID string) domain.QueueEntry {
	return domain.QueueEntry{PlayerID: playerID, DeckID: 303, Name: "carol", Level: 3, JoinedAt: scenarioStartedAt}
}

func pairOfPlayers(firstPlayerID, secondPlayerID string) matchertest.PopStep {
	return matchertest.PairPopped("gw-1", playerEntry(firstPlayerID), playerEntry(secondPlayerID))
}

func pairOfPlayerAAndPlayerB() matchertest.PopStep {
	return matchertest.PairPopped("gw-1", playerA, playerB)
}

func defaultOptions() matcher.Options {
	return matcher.Options{
		Interval:         time.Hour,
		CircuitThreshold: 3,
		CircuitCooldown:  6 * time.Hour,
		DrainTimeout:     10 * time.Second,
	}
}

type scenario struct {
	queue     *matchertest.Queue
	publisher *matchertest.Publisher
	clock     *matchertest.Clock
	logs      *usecasetest.Logs
	matcher   *matcher.Matcher
	interval  time.Duration
	running   *matchertest.Running
}

func newScenario(t *testing.T, opts matcher.Options, startedAt time.Time) *scenario {
	t.Helper()
	logs := usecasetest.CaptureLogs(t)
	clock := matchertest.NewClock(startedAt)
	queue := matchertest.NewQueue()
	publisher := matchertest.NewPublisher()
	m, err := matcher.New(queue, publisher, clock, matchid.NewULIDGenerator(clock), opts)
	require.NoError(t, err)
	return &scenario{queue: queue, publisher: publisher, clock: clock, logs: logs, matcher: m, interval: opts.Interval}
}

func newDefaultScenario(t *testing.T) *scenario {
	t.Helper()
	return newScenario(t, defaultOptions(), scenarioStartedAt)
}

func (s *scenario) start(t *testing.T) {
	t.Helper()
	s.running = matchertest.StartRun(t, s.clock, s.isIdle, s.matcher.Run)
	require.NoError(t, s.clock.WaitForTickers(matchertest.WaitContext(t), 1))
}

func (s *scenario) shutdown() {
	s.running.Shutdown()
}

func (s *scenario) isIdle() bool {
	return !s.queue.HasCallsInFlight() && !s.publisher.HasCallsInFlight()
}

func (s *scenario) activity() int {
	return s.queue.CallCount() + len(s.publisher.Publications()) + s.logs.Count()
}

func (s *scenario) settle(t *testing.T) {
	t.Helper()
	matchertest.WaitUntilQuiet(t, s.activity)
}

func (s *scenario) advance(t *testing.T, d time.Duration) {
	t.Helper()
	s.clock.Advance(d)
	require.NoError(t, s.clock.WaitForTicksConsumed(matchertest.WaitContext(t)))
	s.settle(t)
}

func (s *scenario) tick(t *testing.T) {
	t.Helper()
	s.advance(t, s.interval)
}

func (s *scenario) waitForTimers(t *testing.T, n int) {
	t.Helper()
	require.NoError(t, s.clock.WaitForTimers(matchertest.WaitContext(t), n))
}

func (s *scenario) waitForPublications(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.publisher.Publications()) >= n }, matchertest.WaitLimit, matchertest.PollInterval)
}

func (s *scenario) waitForReenqueues(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(s.queue.ReenqueueRequests()) >= n }, matchertest.WaitLimit, matchertest.PollInterval)
}

func (s *scenario) waitForPops(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return s.queue.PopCalls() >= n }, matchertest.WaitLimit, matchertest.PollInterval)
}

func (s *scenario) waitUntilDone(t *testing.T) {
	t.Helper()
	require.Eventually(t, s.running.IsDone, matchertest.WaitLimit, matchertest.PollInterval)
}

func (s *scenario) circuitState() string {
	return circuitStates[s.matcher.IsCircuitOpen()]
}

func (s *scenario) matchMadeEvent(t *testing.T, index int) apimatchmaking.MatchMadeEvent {
	t.Helper()
	publications := s.publisher.Publications()
	require.Greater(t, len(publications), index)
	var event apimatchmaking.MatchMadeEvent
	require.NoError(t, json.Unmarshal(publications[index].Payload, &event))
	return event
}

func playerIDsOf(event apimatchmaking.MatchMadeEvent) []string {
	playerIDs := make([]string, 0, len(event.Players))
	for _, player := range event.Players {
		playerIDs = append(playerIDs, player.PlayerID)
	}
	return playerIDs
}

func playerIDsOfEntries(entries []domain.QueueEntry) []string {
	playerIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		playerIDs = append(playerIDs, entry.PlayerID)
	}
	return playerIDs
}
