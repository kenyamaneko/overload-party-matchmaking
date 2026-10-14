package redisqueue

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

const queueKey = "matchmaking:queue"

// gatewayInstanceKey は Enqueue が最後に受け取った gatewayInstanceID を保持し、キューのリセット判定に使う。
const gatewayInstanceKey = "matchmaking:gateway_instance_id"

// memberSeparator は ZSET member のフィールドを区切る文字。
const memberSeparator = ":"

// memberFieldCount は ZSET member の `:` 区切りフィールド数 (playerID, deckID, level, base64name)。
const memberFieldCount = 4

// pairInstanceIDLength は PopPair のスクリプトが返す配列の先頭にある gatewayInstanceID の要素数。
const pairInstanceIDLength = 1

// scoredMemberLength は PopPair のスクリプトが返す 1 エントリあたりの要素数 (member, score)。
const scoredMemberLength = 2

// entryScriptArgCount は Reenqueue のスクリプトに渡すエントリ 1 件あたりの引数の数 (playerID, member, score)。
const entryScriptArgCount = 3

var errPlayerIDEmpty = errors.New("playerID is empty")

// RedisQueue は Upstash Redis の Sorted Set を使ったマッチメイキングキューです。
type RedisQueue struct {
	client       *redis.Client
	enqueueLua   *redis.Script
	popPairLua   *redis.Script
	cancelLua    *redis.Script
	reenqueueLua *redis.Script
}

// NewRedisQueue は RedisQueue を生成します。
func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{
		client:       client,
		enqueueLua:   redis.NewScript(enqueueScript),
		popPairLua:   redis.NewScript(popPairScript),
		cancelLua:    redis.NewScript(cancelScript),
		reenqueueLua: redis.NewScript(reenqueueScript),
	}
}

// Enqueue はプレイヤーをキューに追加します。player summary (name / level) を
// queue entry に同梱して保持し、match 成立時の event に伝搬する。
// gatewayInstanceID が保持値と異なる場合はキューを空にしてから登録し、削除した件数を返す。
func (q *RedisQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) (int64, error) {
	if err := validateMemberPlayerID(playerID); err != nil {
		return 0, err
	}
	if gatewayInstanceID == "" {
		return 0, errors.New("gatewayInstanceID is empty")
	}
	score := float64(time.Now().UnixMilli())
	member := encodeMember(playerID, deckID, level, name)
	removed, err := q.enqueueLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}, playerID, member, score, gatewayInstanceID).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis enqueue: %w", err)
	}
	return removed, nil
}

// Cancel はプレイヤーのキューエントリを削除します。
func (q *RedisQueue) Cancel(ctx context.Context, playerID string) (bool, error) {
	if playerID == "" {
		return false, errPlayerIDEmpty
	}
	removed, err := q.cancelLua.Run(ctx, q.client, []string{queueKey}, playerID).Int64()
	if err != nil {
		return false, fmt.Errorf("redis cancel: %w", err)
	}
	return removed > 0, nil
}

// Size はキュー内のエントリ数を返します。
func (q *RedisQueue) Size(ctx context.Context) (int64, error) {
	n, err := q.client.ZCard(ctx, queueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("redis zcard: %w", err)
	}
	return n, nil
}

// PopPair はキュー先頭の 2 件をアトミックに取り出します。取り出した時点の gatewayInstanceID も
// 返す (Reenqueue の書き戻し判定に使う)。
func (q *RedisQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, string, error) {
	values, err := q.popPairLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}).StringSlice()
	if err != nil {
		return nil, "", fmt.Errorf("redis pop pair: %w", err)
	}
	if len(values) == 0 {
		return nil, "", nil
	}

	gatewayInstanceID := values[0]
	raw := values[pairInstanceIDLength:]
	entries := make([]domain.QueueEntry, 0, len(raw)/scoredMemberLength)
	for i := 0; i < len(raw); i += scoredMemberLength {
		playerID, deckID, level, name, err := decodeMember(raw[i])
		if err != nil {
			return nil, "", err
		}
		scoreMillis, err := strconv.ParseInt(raw[i+1], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("redis pop pair: parse score: %w", err)
		}
		entries = append(entries, domain.QueueEntry{
			PlayerID: playerID,
			DeckID:   deckID,
			Name:     name,
			Level:    level,
			JoinedAt: time.UnixMilli(scoreMillis),
		})
	}
	return entries, gatewayInstanceID, nil
}

// Reenqueue はエントリを元の JoinedAt スコアでキューに再追加します。gatewayInstanceID には
// entries を取り出した時点で PopPair が返した値を渡し、現在の保持値と一致する場合のみ書き戻して true を返す。
// entries が空でも識別子の一致を判定する。同じ playerID の既存エントリは取り除いてから書く。
// member 文字列は Go 側で組み立てて Lua に渡す (Lua から encoding 知識を排除する設計)。
func (q *RedisQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry, gatewayInstanceID string) (bool, error) {
	args := make([]any, 0, len(entries)*entryScriptArgCount+1)
	args = append(args, gatewayInstanceID)
	for _, e := range entries {
		if err := validateMemberPlayerID(e.PlayerID); err != nil {
			return false, err
		}
		args = append(args, e.PlayerID, encodeMember(e.PlayerID, e.DeckID, e.Level, e.Name), float64(e.JoinedAt.UnixMilli()))
	}
	written, err := q.reenqueueLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}, args...).Int64()
	if err != nil {
		return false, fmt.Errorf("redis reenqueue: %w", err)
	}
	return written > 0, nil
}

// validateMemberPlayerID は member に保存できる playerID かを検査する。
// `:` を含むと member の先頭フィールドの境界が曖昧になり、別のプレイヤーのエントリと取り違えるため拒否する。
func validateMemberPlayerID(playerID string) error {
	if playerID == "" {
		return errPlayerIDEmpty
	}
	if strings.Contains(playerID, memberSeparator) {
		return errors.New("playerID contains ':'")
	}
	return nil
}

// encodeMember は ZSET member の文字列表現を組み立てる。
// 形式: playerID:deckID:level:<base64(name)>
// name は UI 自由入力で `:` を含む可能性があるため base64 (no padding) で encode し、
// 区切り文字との衝突を避ける。
func encodeMember(playerID string, deckID, level int64, name string) string {
	return fmt.Sprintf("%s:%d:%d:%s", playerID, deckID, level, base64.RawStdEncoding.EncodeToString([]byte(name)))
}

func decodeMember(member string) (string, int64, int64, string, error) {
	parts := strings.SplitN(member, memberSeparator, memberFieldCount)
	if len(parts) != memberFieldCount {
		return "", 0, 0, "", fmt.Errorf("decode member: expected %d fields, got %d in %q", memberFieldCount, len(parts), member)
	}
	deckID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, 0, "", fmt.Errorf("decode member: parse deck: %w", err)
	}
	level, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", 0, 0, "", fmt.Errorf("decode member: parse level: %w", err)
	}
	nameBytes, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", 0, 0, "", fmt.Errorf("decode member: parse name: %w", err)
	}
	return parts[0], deckID, level, string(nameBytes), nil
}
