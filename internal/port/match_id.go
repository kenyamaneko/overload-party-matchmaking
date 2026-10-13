package port

// MatchIDGenerator はマッチの識別子を発行する interface です。
type MatchIDGenerator interface {
	NewMatchID() string
}
