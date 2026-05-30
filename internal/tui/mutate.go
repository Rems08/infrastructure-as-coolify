package tui

import "github.com/Rems08/infrastructure-as-coolify/internal/apply"

// recorder is the persistent audit sink for lifecycle actions. It is the append-only log
// the apply and destroy commands already write to, narrowed to the one method the browser
// needs (accept interfaces); *apply.Auditor satisfies it.
type recorder interface {
	Record(apply.AuditEntry) error
}
