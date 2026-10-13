package httphandlertest

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"
)

// LogRecord は捕捉した構造化ログの 1 件。
type LogRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

// LogCapture は標準の構造化ログへ出力されたレコードを保持する。
type LogCapture struct {
	mutex   sync.Mutex
	records []LogRecord
}

// CaptureLogs は slog の既定のロガーの出力を捕捉し、テストの終了時に元のロガーへ戻す。並列に実行するテストでは使えない。
func CaptureLogs(t *testing.T) *LogCapture {
	t.Helper()
	capture := &LogCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(&captureHandler{capture: capture}))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return capture
}

// RecordsAt は level のレコードを出力順に返す。
func (c *LogCapture) RecordsAt(level slog.Level) []LogRecord {
	return c.recordsExcluding(func(record LogRecord) bool { return record.Level != level })
}

// RecordsWithMessage は message のレコードを出力順に返す。
func (c *LogCapture) RecordsWithMessage(message string) []LogRecord {
	return c.recordsExcluding(func(record LogRecord) bool { return record.Message != message })
}

func (c *LogCapture) recordsExcluding(isExcluded func(LogRecord) bool) []LogRecord {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return slices.DeleteFunc(slices.Clone(c.records), isExcluded)
}

func (c *LogCapture) append(record LogRecord) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.records = append(c.records, record)
}

type captureHandler struct {
	capture *LogCapture
	attrs   []slog.Attr
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]string)
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value.String()
	}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.String()
		return true
	})
	h.capture.append(LogRecord{Level: record.Level, Message: record.Message, Attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureHandler{capture: h.capture, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *captureHandler) WithGroup(string) slog.Handler { return h }
