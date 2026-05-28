// Package state holds the optional UUID resolver cache. By contract it NEVER stores
// secrets: MarshalJSON refuses to serialise any value that contains a
// secrets.Secret field (belt-and-braces ratchet C-L0.4, threat-model T-S1.3).
package state

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// secretType is the reflect.Type guarded against in the State cache.
var secretType = reflect.TypeOf(secrets.Secret{})

// State is the on-disk UUID resolver cache (opt-in via --state-cache). It maps logical
// resource keys to Coolify UUIDs and records spec integrity metadata. It carries NO
// secret by design.
type State struct {
	UUIDs       map[string]string `json:"uuids"`
	ResolvedAt  time.Time         `json:"resolved_at"`
	OpenAPIHash string            `json:"openapi_hash"`
}

// MarshalJSON serialises the State after asserting it holds no Secret field. If a
// future change introduces one, marshalling fails loudly instead of leaking it.
func (s *State) MarshalJSON() ([]byte, error) {
	if containsSecret(reflect.TypeOf(*s)) {
		return nil, fmt.Errorf("state: BUG — secrets.Secret field detected, refusing to marshal")
	}
	type alias State // avoid recursing into this MarshalJSON
	return json.Marshal((*alias)(s))
}

// containsSecret reports whether t is, contains, or transitively references a
// secrets.Secret field.
func containsSecret(t reflect.Type) bool {
	return hasSecret(t, make(map[reflect.Type]bool))
}

func hasSecret(t reflect.Type, seen map[reflect.Type]bool) bool {
	if t == secretType {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return hasSecret(t.Elem(), seen)
	case reflect.Map:
		return hasSecret(t.Key(), seen) || hasSecret(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if hasSecret(t.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}
