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

// gatewayInstanceKey は Enqueue が最後に受け取った gatewayInstanceID を保持する。
// 保持値と異なる gatewayInstanceID を受け取ったキューのリセット判定に使う。
const gatewayInstanceKey = "matchmaking:gateway_instance_id"

// memberFieldCount は ZSET member の `:` 区切りフィールド数 (playerID, deckID, level, base64name)。
const memberFieldCount = 4

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
// gatewayInstanceID が直前の Enqueue と異なる場合、別プロセスへの切り替わりとみなし
// キュー全体を空にしてから登録する。戻り値はこのリセットで削除した件数。
func (q *RedisQueue) Enqueue(ctx context.Context, playerID string, deckID int64, name string, level int64, gatewayInstanceID string) (int64, error) {
	if playerID == "" {
		return 0, errors.New("playerID is empty")
	}
	if gatewayInstanceID == "" {
		return 0, errors.New("gatewayInstanceID is empty")
	}
	score := float64(time.Now().UnixMilli())
	member := encodeMember(playerID, deckID, level, name)
	res, err := q.enqueueLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}, playerID, member, score, gatewayInstanceID).Result()
	if err != nil {
		return 0, fmt.Errorf("redis enqueue: %w", err)
	}
	removed, ok := res.(int64)
	if !ok {
		return 0, fmt.Errorf("redis enqueue: unexpected return type %T", res)
	}
	return removed, nil
}

// Cancel はプレイヤーのキューエントリを削除します。
func (q *RedisQueue) Cancel(ctx context.Context, playerID string) (bool, error) {
	if playerID == "" {
		return false, errors.New("playerID is empty")
	}
	res, err := q.cancelLua.Run(ctx, q.client, []string{queueKey}, playerID).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false, fmt.Errorf("redis cancel: %w", err)
	}
	removed, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("redis cancel: unexpected return type %T", res)
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

// PopPair はキュー先頭の 2 件をアトミックに取り出します。取り出した時点で保持していた
// gatewayInstanceID も合わせて返す (Reenqueue に渡し、書き戻し可否の判定に使う)。
func (q *RedisQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, string, error) {
	res, err := q.popPairLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("redis pop pair: %w", err)
	}

	outer, ok := res.([]any)
	if !ok {
		return nil, "", fmt.Errorf("redis pop pair: unexpected return type %T", res)
	}
	if len(outer) == 0 {
		return nil, "", nil
	}
	if len(outer) != 2 {
		return nil, "", fmt.Errorf("redis pop pair: unexpected element count %d", len(outer))
	}
	gatewayInstanceID, ok := outer[0].(string)
	if !ok {
		return nil, "", fmt.Errorf("redis pop pair: instance id not string")
	}
	raw, ok := outer[1].([]any)
	if !ok {
		return nil, "", fmt.Errorf("redis pop pair: pair not array")
	}
	if len(raw)%2 != 0 {
		return nil, "", fmt.Errorf("redis pop pair: odd element count %d", len(raw))
	}

	entries := make([]domain.QueueEntry, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		memberStr, ok := raw[i].(string)
		if !ok {
			return nil, "", fmt.Errorf("redis pop pair: member %d not string", i)
		}
		scoreStr, ok := raw[i+1].(string)
		if !ok {
			return nil, "", fmt.Errorf("redis pop pair: score %d not string", i+1)
		}
		playerID, deckID, level, name, err := decodeMember(memberStr)
		if err != nil {
			return nil, "", err
		}
		scoreMillis, err := strconv.ParseInt(scoreStr, 10, 64)
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

// Reenqueue はエントリを元の JoinedAt スコアでキューに再追加します。gatewayInstanceID は
// entries を取り出した時点で PopPair が返した値を渡す。現在保持している値と一致しない場合は
// 別プロセスへの切り替わりが起きたとみなし書き戻さず false を返す。
// member 文字列は Go 側で組み立てて Lua に渡す (Lua から encoding 知識を排除する設計)。
func (q *RedisQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry, gatewayInstanceID string) (bool, error) {
	if len(entries) == 0 {
		return true, nil
	}
	args := make([]any, 0, len(entries)*2+1)
	args = append(args, gatewayInstanceID)
	for _, e := range entries {
		args = append(args, encodeMember(e.PlayerID, e.DeckID, e.Level, e.Name), float64(e.JoinedAt.UnixMilli()))
	}
	res, err := q.reenqueueLua.Run(ctx, q.client, []string{queueKey, gatewayInstanceKey}, args...).Result()
	if err != nil {
		return false, fmt.Errorf("redis reenqueue: %w", err)
	}
	written, ok := res.(int64)
	if !ok {
		return false, fmt.Errorf("redis reenqueue: unexpected return type %T", res)
	}
	return written > 0, nil
}

// encodeMember は ZSET member の文字列表現を組み立てる。
// 形式: playerID:deckID:level:<base64(name)>
// name は UI 自由入力で `:` を含む可能性があるため base64 (no padding) で encode し、
// 区切り文字との衝突を避ける。
func encodeMember(playerID string, deckID, level int64, name string) string {
	return fmt.Sprintf("%s:%d:%d:%s", playerID, deckID, level, base64.RawStdEncoding.EncodeToString([]byte(name)))
}

func decodeMember(member string) (string, int64, int64, string, error) {
	parts := strings.SplitN(member, ":", memberFieldCount)
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
