// Package plan computes the semantic difference between the desired state (YAML config)
// and the actual state (live Coolify), rendering it Terraform-style. The diff is
// per-field and secret-aware: secret values follow the Notify-only policy and never
// appear, in any form, in the output.
package plan

import (
	"fmt"

	"github.com/google/go-cmp/cmp"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// Op is the kind of a change.
type Op string

const (
	// OpAdd is a field present in the desired state but absent in the actual state.
	OpAdd Op = "add"
	// OpUpdate is a field whose value differs between desired and actual.
	OpUpdate Op = "update"
	// OpDelete is a field present in the actual state but absent in the desired state.
	OpDelete Op = "delete"
)

// Value is a diffable field value: either a visible scalar or an opaque secret. Exactly
// one form is meaningful, selected by isSecret.
type Value struct {
	scalar   string
	secret   secrets.Secret
	isSecret bool
}

// Scalar builds a visible field value.
func Scalar(s string) Value { return Value{scalar: s} }

// SecretValue builds an opaque field value whose content is never displayed.
func SecretValue(s secrets.Secret) Value { return Value{secret: s, isSecret: true} }

// display returns a render-safe representation: a scalar verbatim, a secret as its source
// declaration (safe to show) or [REDACTED] when it has none.
func (v Value) display() string {
	if !v.isSecret {
		return v.scalar
	}
	if origin := v.secret.Origin(); origin != "" {
		return origin
	}
	return v.secret.String()
}

// Field is one semantic field of a resource; declaration order is preserved for output.
type Field struct {
	Name  string
	Value Value
}

// Resource is a normalized, diffable snapshot of one managed resource.
type Resource struct {
	Kind   string
	Name   string
	Fields []Field
}

// Change is a single field-level difference. Old and New are display-safe: for secrets
// they carry only the source declaration or a redacted note, never the value.
type Change struct {
	Op        Op     `json:"op"`
	Path      string `json:"path"`
	Old       string `json:"old,omitempty"`
	New       string `json:"new,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// Diff returns the field-level changes that turn actual into desired. A nil actual means
// the resource does not exist yet, so every desired field is an addition. Fields present
// in actual but not desired are reported as deletions.
func Diff(desired Resource, actual *Resource) []Change {
	prefix := desired.Kind + "." + desired.Name
	if actual == nil {
		changes := make([]Change, 0, len(desired.Fields))
		for _, f := range desired.Fields {
			changes = append(changes, Change{
				Op:        OpAdd,
				Path:      prefix + "." + f.Name,
				New:       f.Value.display(),
				Sensitive: f.Value.isSecret,
			})
		}
		return changes
	}

	actualByName := make(map[string]Value, len(actual.Fields))
	for _, f := range actual.Fields {
		actualByName[f.Name] = f.Value
	}
	desiredSeen := make(map[string]bool, len(desired.Fields))

	var changes []Change
	for _, df := range desired.Fields {
		desiredSeen[df.Name] = true
		path := prefix + "." + df.Name
		av, ok := actualByName[df.Name]
		if !ok {
			changes = append(changes, Change{Op: OpAdd, Path: path, New: df.Value.display(), Sensitive: df.Value.isSecret})
			continue
		}
		if c, changed := diffValue(path, av, df.Value); changed {
			changes = append(changes, c)
		}
	}
	for _, af := range actual.Fields {
		if !desiredSeen[af.Name] {
			changes = append(changes, Change{Op: OpDelete, Path: prefix + "." + af.Name, Old: af.Value.display(), Sensitive: af.Value.isSecret})
		}
	}
	return changes
}

// diffValue compares an actual (old) and desired (new) value, returning the change and
// whether they differ.
func diffValue(path string, old, newv Value) (Change, bool) {
	switch {
	case old.isSecret && newv.isSecret:
		return diffSecret(path, old.secret, newv.secret)
	case old.isSecret != newv.isSecret:
		// The field flipped between visible and secret; surface it without leaking.
		return Change{Op: OpUpdate, Path: path, Old: old.display(), New: newv.display(), Sensitive: true}, true
	default:
		if cmp.Equal(old.scalar, newv.scalar) {
			return Change{}, false
		}
		return Change{Op: OpUpdate, Path: path, Old: old.scalar, New: newv.scalar}, true
	}
}

// diffSecret implements the Notify-only secret diff: only the source declaration is ever
// shown; a value change with an unchanged source is announced
// without exposing the value or any hash.
func diffSecret(path string, old, newv secrets.Secret) (Change, bool) {
	if old.Origin() != newv.Origin() {
		return Change{Op: OpUpdate, Path: path, Old: old.Origin(), New: newv.Origin(), Sensitive: true}, true
	}
	// A desired secret loaded read-only carries no value (its ${env:} reference is bound only
	// at apply). The source is unchanged, so comparing the absent value against the remote one
	// would report a phantom "resolved value changed"; with the same origin it is a no-op.
	if newv.IsUnresolvedEnv() {
		return Change{}, false
	}
	if old.ValueEquals(newv) {
		return Change{}, false
	}
	return Change{
		Op:        OpUpdate,
		Path:      path,
		New:       fmt.Sprintf("(resolved value changed, source %s unchanged)", newv.Origin()),
		Sensitive: true,
	}, true
}
