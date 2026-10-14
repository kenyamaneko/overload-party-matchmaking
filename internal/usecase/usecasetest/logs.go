// Package usecasetest は usecase のテストが出力されたログを読み取るための部品を提供する。
package usecasetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
)

// Logs は slog の既定のロガーへ出力されたログを JSON として記録します。
type Logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// LogEntry は記録された 1 件のログです。
type LogEntry struct {
	attrs map[string]any
}

// CaptureLogs は slog の既定のロガーの出力を記録し始め、テストの終了時に元のロガーへ戻します。
func CaptureLogs(t *testing.T) *Logs {
	t.Helper()
	logs := &Logs{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

// Write は出力された 1 行を記録します。
func (l *Logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// Count は記録したログの件数を返します。
func (l *Logs) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Count(l.buf.Bytes(), []byte("\n"))
}

// Entries は level と message が一致するログを出力された順に返します。
func (l *Logs) Entries(level slog.Level, message string) []LogEntry {
	l.mu.Lock()
	lines := bytes.Split(bytes.TrimSpace(l.buf.Bytes()), []byte("\n"))
	l.mu.Unlock()
	var entries []LogEntry
	for _, line := range lines {
		attrs := map[string]any{}
		if err := json.Unmarshal(line, &attrs); err != nil {
			continue
		}
		if attrs["level"] == level.String() && attrs["msg"] == message {
			entries = append(entries, LogEntry{attrs: attrs})
		}
	}
	return entries
}

// Text は項目 key の値を文字列にして返します。
func (e LogEntry) Text(key string) string {
	return fmt.Sprint(e.attrs[key])
}

// Texts は配列の項目 key の要素を文字列にして順に返します。
func (e LogEntry) Texts(key string) []string {
	values, _ := e.attrs[key].([]any)
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, fmt.Sprint(value))
	}
	return texts
}
