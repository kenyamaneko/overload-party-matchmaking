package apimatchmaking

// マッチ成立通知の Pub/Sub トピック名。
// gateway が Exactly-Once Delivery 前提で購読する単一チャネル。
const TopicMatchmakingEvents = "matchmaking-events"

// MatchMadeEvent の Type 値。
const EventTypeMatchMade = "match_made"
