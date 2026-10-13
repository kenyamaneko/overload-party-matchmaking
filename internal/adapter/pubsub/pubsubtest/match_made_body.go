package pubsubtest

import "fmt"

// BuildMatchMadeBody は指定したマッチ ID を持つ match_made の本文を組み立てる。
func BuildMatchMadeBody(matchID string) string {
	return fmt.Sprintf(`{"event_type":"match_made","match_id":%q,"players":[{"player_id":"tst-player-a","deck_id":1,"name":"tst-name-a","level":1},{"player_id":"tst-player-b","deck_id":2,"name":"tst-name-b","level":2}]}`, matchID)
}
