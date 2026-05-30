package tui

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// collector captures the messages a handler sends, mimicking program.Send.
type collector struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (c *collector) send(m tea.Msg) {
	c.mu.Lock()
	c.msgs = append(c.msgs, m)
	c.mu.Unlock()
}

func TestLogHandler_SendsRecordAsMsg(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	c := &collector{}
	h.Wire(c.send)

	log := slog.New(h)
	log.Info("resolved databases", "resolved", 3)

	if len(c.msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(c.msgs))
	}
	msg, ok := c.msgs[0].(LogMsg)
	if !ok {
		t.Fatalf("msg type = %T, want LogMsg", c.msgs[0])
	}
	if msg.Message != "resolved databases" {
		t.Errorf("message = %q", msg.Message)
	}
	if !strings.Contains(msg.Attrs, "resolved=3") {
		t.Errorf("attrs = %q, want resolved=3", msg.Attrs)
	}
}

func TestLogHandler_DropsBeforeWire(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	// No Wire: Handle must be a no-op, not a panic.
	if err := h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "x", 0)); err != nil {
		t.Fatalf("handle before wire: %v", err)
	}
}

func TestLogHandler_RedactsSecretAttr(t *testing.T) {
	h := NewLogHandler(slog.LevelInfo)
	c := &collector{}
	h.Wire(c.send)

	sec := secrets.NewRemote("super-secret-value")
	slog.New(h).Info("token loaded", "token", sec)

	msg := c.msgs[0].(LogMsg)
	if strings.Contains(msg.Attrs, "super-secret-value") {
		t.Fatalf("attrs leaked secret value: %q", msg.Attrs)
	}
	if !strings.Contains(msg.Attrs, "REDACTED") {
		t.Errorf("attrs = %q, want [REDACTED]", msg.Attrs)
	}
}

func TestLogHandler_LevelFilter(t *testing.T) {
	h := NewLogHandler(slog.LevelWarn)
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info enabled under warn threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("error not enabled under warn threshold")
	}
}
