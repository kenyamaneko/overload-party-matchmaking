// Package redisqueue は port.Queue の Redis 実装を提供する。
// Lua スクリプトによる atomic な enqueue / pop 操作で重複マッチングを防ぐ。
package redisqueue
