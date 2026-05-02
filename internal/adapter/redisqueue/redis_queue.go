package redisqueue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kenyamaneko/overload-party-matchmaking/internal/domain"
)

const queueKey = "matchmaking:queue"

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

// Enqueue はプレイヤーをキューに追加します。
func (q *RedisQueue) Enqueue(ctx context.Context, playerID string, deckID int64) error {
	if playerID == "" {
		return errors.New("playerID is empty")
	}
	score := float64(time.Now().UnixMilli())
	_, err := q.enqueueLua.Run(ctx, q.client, []string{queueKey}, playerID, deckID, score).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis enqueue: %w", err)
	}
	return nil
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

// PopPair はキュー先頭の 2 件をアトミックに取り出します。
func (q *RedisQueue) PopPair(ctx context.Context) ([]domain.QueueEntry, error) {
	res, err := q.popPairLua.Run(ctx, q.client, []string{queueKey}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis pop pair: %w", err)
	}

	raw, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("redis pop pair: unexpected return type %T", res)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("redis pop pair: odd element count %d", len(raw))
	}

	entries := make([]domain.QueueEntry, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		memberStr, ok := raw[i].(string)
		if !ok {
			return nil, fmt.Errorf("redis pop pair: member %d not string", i)
		}
		scoreStr, ok := raw[i+1].(string)
		if !ok {
			return nil, fmt.Errorf("redis pop pair: score %d not string", i+1)
		}
		playerID, deckID, err := decodeMember(memberStr)
		if err != nil {
			return nil, err
		}
		scoreMillis, err := strconv.ParseInt(scoreStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("redis pop pair: parse score: %w", err)
		}
		entries = append(entries, domain.QueueEntry{
			PlayerID: playerID,
			DeckID:   deckID,
			JoinedAt: time.UnixMilli(scoreMillis),
		})
	}
	return entries, nil
}

// Reenqueue はエントリを元の JoinedAt スコアでキューに再追加します。
func (q *RedisQueue) Reenqueue(ctx context.Context, entries []domain.QueueEntry) error {
	if len(entries) == 0 {
		return nil
	}
	args := make([]any, 0, len(entries)*3)
	for _, e := range entries {
		args = append(args, e.PlayerID, e.DeckID, float64(e.JoinedAt.UnixMilli()))
	}
	_, err := q.reenqueueLua.Run(ctx, q.client, []string{queueKey}, args...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("redis reenqueue: %w", err)
	}
	return nil
}

func decodeMember(member string) (string, int64, error) {
	for i := len(member) - 1; i >= 0; i-- {
		if member[i] == ':' {
			playerID := member[:i]
			deckID, err := strconv.ParseInt(member[i+1:], 10, 64)
			if err != nil {
				return "", 0, fmt.Errorf("decode member: parse deck: %w", err)
			}
			return playerID, deckID, nil
		}
	}
	return "", 0, fmt.Errorf("decode member: missing separator in %q", member)
}
