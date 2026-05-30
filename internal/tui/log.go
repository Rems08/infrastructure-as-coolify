package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogMsg carries one slog record into the Bubble Tea update loop. Routing log records
// through the message loop (rather than a buffer shared with the render goroutine) keeps
// the log pane race-free: every record is appended from Update, which is single-threaded.
type LogMsg struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   string
}

// LogHandler is an slog.Handler that forwards each record to a Bubble Tea program via
// Send. The program reference is wired after the program is constructed, so Send is
// guarded by a mutex; records logged before wiring (there are none in practice) are
// dropped rather than panicking.
//
// Attribute values are formatted with their own String methods, so a secrets.Secret stays
// "[REDACTED]" here exactly as it does everywhere else; the handler never reveals a value.
type LogHandler struct {
	mu    *sync.Mutex
	send  *func(tea.Msg)
	level slog.Level
	attrs []slog.Attr
}

// NewLogHandler returns a handler whose Send target is set later with Wire.
func NewLogHandler(level slog.Level) *LogHandler {
	var send func(tea.Msg)
	return &LogHandler{mu: &sync.Mutex{}, send: &send, level: level}
}

// Wire points the handler at a program's Send. It is safe to call from the goroutine that
// creates the program while records may already be arriving on command goroutines.
func (h *LogHandler) Wire(send func(tea.Msg)) {
	h.mu.Lock()
	*h.send = send
	h.mu.Unlock()
}

func (h *LogHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *LogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	send := *h.send
	h.mu.Unlock()
	if send == nil {
		return nil
	}
	send(LogMsg{Time: r.Time, Level: r.Level, Message: r.Message, Attrs: h.formatAttrs(r)})
	return nil
}

func (h *LogHandler) formatAttrs(r slog.Record) string {
	var b strings.Builder
	for _, a := range h.attrs {
		appendAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, a)
		return true
	})
	return strings.TrimSpace(b.String())
}

// appendAttr resolves the attribute first so a slog.LogValuer (such as secrets.Secret,
// which resolves to "[REDACTED]") is honoured rather than the raw underlying value.
func appendAttr(b *strings.Builder, a slog.Attr) {
	fmt.Fprintf(b, "%s=%v ", a.Key, a.Value.Resolve())
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &next
}

func (h *LogHandler) WithGroup(string) slog.Handler { return h }
