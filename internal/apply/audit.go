package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry is one append-only audit-log line. It records what was applied and the
// sources of any secrets involved — never their resolved values.
type AuditEntry struct {
	Time      string   `json:"time"`
	Operation string   `json:"operation"` // "apply"
	Resource  string   `json:"resource"`  // e.g. "Application/beenaire/staging/api"
	Op        string   `json:"op"`        // create | update | delete
	Sources   []string `json:"sources,omitempty"`
	DiffHash  string   `json:"diff_hash,omitempty"`
	// ComposeHash is sha256 of a Service's decoded docker-compose content. The content
	// itself is never logged: a compose file can hold inline secrets, and the audit log is
	// a plain local file an operator might commit by accident.
	ComposeHash string `json:"compose_hash,omitempty"`
}

// Auditor appends audit entries to a local, append-only log file created 0600 (the run is
// local to one operator's machine). The clock is injectable for deterministic tests.
type Auditor struct {
	path string
	now  func() time.Time
}

// NewAuditor returns an Auditor writing to path. The parent directory is created on first
// write. A zero or empty path is a programming error; callers gate on a configured path.
func NewAuditor(path string) *Auditor {
	return &Auditor{path: path, now: time.Now}
}

// Record appends e as one JSON line. It stamps e.Time when unset, creates the parent
// directory and file (0600) if needed, and never truncates an existing log.
func (a *Auditor) Record(e AuditEntry) error {
	if e.Time == "" {
		e.Time = a.now().UTC().Format(time.RFC3339)
	}
	if e.Operation == "" {
		e.Operation = "apply"
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("audit: marshal entry: %w", err)
	}
	if dir := filepath.Dir(a.path); dir != "" {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Errorf("audit: mkdir: %w", mkErr)
		}
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open log: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("audit: write: %w", err)
	}
	return nil
}
