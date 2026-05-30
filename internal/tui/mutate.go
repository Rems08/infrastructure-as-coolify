package tui

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// Lifecycle action names. They double as the audit-log operation and the slog message verb.
const (
	actionStart   = "start"
	actionStop    = "stop"
	actionRestart = "restart"
)

// recorder is the persistent audit sink for lifecycle actions. It is the append-only log
// the apply and destroy commands already write to, narrowed to the one method the browser
// needs (accept interfaces); *apply.Auditor satisfies it.
type recorder interface {
	Record(apply.AuditEntry) error
}

// confirmState is an active confirmation prompt. While it is set, every key press is routed
// to it (so an accidental key cannot trigger or escape a mutation): only y confirms, n/esc
// cancels, and onConfirm is the command run on confirmation — built ahead of time so the
// mutation itself never runs inline in Update.
type confirmState struct {
	prompt    string
	onConfirm tea.Cmd
}

// mutationDoneMsg reports the outcome of a lifecycle action back to the update loop.
type mutationDoneMsg struct {
	action string
	name   string
	err    error
}

// lifecycleCmd returns a command that performs a lifecycle action off the update loop and
// traces it. The mutation never runs inline: it is wrapped in the returned tea.Cmd.
func lifecycleCmd(ctx context.Context, mut mutatorClient, aud recorder, action string, key state.ResourceKey, uuid string) tea.Cmd {
	return func() tea.Msg {
		err := applyLifecycle(ctx, mut, action, uuid)
		traceMutation(ctx, aud, action, key, err)
		return mutationDoneMsg{action: action, name: key.Name, err: err}
	}
}

func applyLifecycle(ctx context.Context, mut mutatorClient, action, uuid string) error {
	switch action {
	case actionStart:
		return mut.StartApplication(ctx, uuid)
	case actionStop:
		return mut.StopApplication(ctx, uuid)
	case actionRestart:
		return mut.RestartApplication(ctx, uuid)
	default:
		return fmt.Errorf("unknown lifecycle action %q", action)
	}
}

// traceMutation emits a structured record for every lifecycle action: always an slog record
// (surfaced in the log pane), and — when an auditor is wired — an append-only audit entry.
// Neither carries any secret; the audit entry records only the action and resource identity.
func traceMutation(ctx context.Context, aud recorder, action string, key state.ResourceKey, err error) {
	if err != nil {
		slog.ErrorContext(ctx, "application "+action+" failed", "resource", key.Name, "error", err)
		return
	}
	slog.InfoContext(ctx, "application "+action, "resource", key.Name)
	if aud == nil {
		return
	}
	entry := apply.AuditEntry{
		Operation: action,
		Resource:  "Application/" + key.Project + "/" + key.Environment + "/" + key.Name,
	}
	if recErr := aud.Record(entry); recErr != nil {
		slog.WarnContext(ctx, "audit record failed", "error", recErr)
	}
}
