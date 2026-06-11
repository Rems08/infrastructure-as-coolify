package plan

import (
	"fmt"
	"strings"
)

// Action is the resource-level outcome of a diff.
type Action string

const (
	// ActionCreate marks a resource that does not exist remotely yet.
	ActionCreate Action = "create"
	// ActionUpdate marks a resource whose fields differ from the remote state.
	ActionUpdate Action = "update"
	// ActionNoop marks a resource already in the desired state.
	ActionNoop Action = "noop"
)

// Item is one resource's planned change. RequiresRecreate is set when at least one change
// cannot be patched in place (a destination move): the resource keeps Action update in the
// summary counts, but converging it takes a destroy followed by a fresh apply.
type Item struct {
	Kind             string   `json:"kind"`
	Name             string   `json:"name"`
	Action           Action   `json:"action"`
	RequiresRecreate bool     `json:"requires_recreate"`
	Changes          []Change `json:"changes"`
}

// Plan aggregates the per-resource diffs of a run.
type Plan struct {
	Items []Item `json:"items"`
}

// Add diffs one resource (a nil actual means it does not exist remotely) and records it.
func (p *Plan) Add(desired Resource, actual *Resource) {
	changes := Diff(desired, actual)
	action := ActionNoop
	switch {
	case actual == nil:
		action = ActionCreate
	case len(changes) > 0:
		action = ActionUpdate
	}
	recreate := false
	for _, c := range changes {
		if c.RequiresRecreate {
			recreate = true
			break
		}
	}
	p.Items = append(p.Items, Item{Kind: desired.Kind, Name: desired.Name, Action: action, RequiresRecreate: recreate, Changes: changes})
}

// Summary counts resources by outcome (Terraform-style).
type Summary struct {
	Add     int `json:"add"`
	Change  int `json:"change"`
	Destroy int `json:"destroy"`
}

// Summary returns the resource-level counts.
func (p Plan) Summary() Summary {
	var s Summary
	for _, it := range p.Items {
		switch it.Action {
		case ActionCreate:
			s.Add++
		case ActionUpdate:
			s.Change++
		case ActionNoop:
		}
	}
	return s
}

// HasChanges reports whether any resource is created or updated (drives the
// --detailed-exitcode flag: true → exit 2).
func (p Plan) HasChanges() bool {
	for _, it := range p.Items {
		if it.Action != ActionNoop {
			return true
		}
	}
	return false
}

// Output is the machine-readable plan (--output=json). The top-level changes list is a
// flattened view for tooling; items preserve per-resource grouping.
type Output struct {
	Changes []Change `json:"changes"`
	Items   []Item   `json:"items"`
	Summary Summary  `json:"summary"`
}

// Output builds the JSON view, never nil-typed so `jq` sees an array.
func (p Plan) Output() Output {
	changes := []Change{}
	for _, it := range p.Items {
		changes = append(changes, it.Changes...)
	}
	return Output{Changes: changes, Items: p.Items, Summary: p.Summary()}
}

// RenderText renders the plan Terraform-style.
func (p Plan) RenderText() string {
	var b strings.Builder
	for _, it := range p.Items {
		switch it.Action {
		case ActionNoop:
			fmt.Fprintf(&b, "  %s.%s: no changes\n", it.Kind, it.Name)
			continue
		case ActionCreate:
			fmt.Fprintf(&b, "+ %s.%s will be created\n", it.Kind, it.Name)
		case ActionUpdate:
			if it.RequiresRecreate {
				fmt.Fprintf(&b, "-/+ %s.%s must be recreated (%s)\n", it.Kind, it.Name, recreateNote(it.Changes))
			} else {
				fmt.Fprintf(&b, "~ %s.%s will be updated\n", it.Kind, it.Name)
			}
		}
		for _, c := range it.Changes {
			b.WriteString(renderChange(c))
		}
	}
	s := p.Summary()
	fmt.Fprintf(&b, "\nPlan: %d to add, %d to change, %d to destroy.\n", s.Add, s.Change, s.Destroy)
	return b.String()
}

// recreateNote summarises why a resource must be recreated: each non-patchable change
// (destination move) is listed by its last path segment with its old and new value, e.g.
// `destination changed: server "localhost" -> "hetzner-1"`.
func recreateNote(changes []Change) string {
	var parts []string
	for _, c := range changes {
		if !c.RequiresRecreate {
			continue
		}
		label := c.Path
		if i := strings.LastIndex(label, "."); i >= 0 {
			label = label[i+1:]
		}
		parts = append(parts, fmt.Sprintf("%s %q -> %q", label, c.Old, c.New))
	}
	return "destination changed: " + strings.Join(parts, ", ")
}

func renderChange(c Change) string {
	switch c.Op {
	case OpAdd:
		return fmt.Sprintf("    + %s = %s\n", c.Path, renderVal(c.New, c.Sensitive))
	case OpDelete:
		return fmt.Sprintf("    - %s = %s\n", c.Path, renderVal(c.Old, c.Sensitive))
	case OpUpdate:
		switch {
		case c.Sensitive && c.Old == "":
			return fmt.Sprintf("    ~ %s: %s\n", c.Path, c.New)
		case c.Sensitive:
			return fmt.Sprintf("    ~ %s: %s -> %s\n", c.Path, c.Old, c.New)
		default:
			return fmt.Sprintf("    ~ %s: %q -> %q\n", c.Path, c.Old, c.New)
		}
	default:
		return ""
	}
}

// renderVal quotes visible scalars; secret displays (a source declaration or note) are
// shown verbatim and never quoted.
func renderVal(v string, sensitive bool) string {
	if sensitive {
		return v
	}
	return fmt.Sprintf("%q", v)
}
