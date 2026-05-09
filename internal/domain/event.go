package domain

// MatchedPlayer はマッチに含まれる 1 プレイヤーの内部表現。
type MatchedPlayer struct {
	PlayerID string
	DeckID   int64
}

// MatchMadeEvent はペア成立の業務事実を表す内部 event。
// wire 変換 (event_type discriminator など) は presenter 層で付与する。
type MatchMadeEvent struct {
	MatchID string
	Players []MatchedPlayer
}
