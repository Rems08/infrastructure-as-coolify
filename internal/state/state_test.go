package state

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// TestStateMarshalRefusesSecretField asserts the marshal guard fires if State ever
// regresses. The real State has no Secret field, so it marshals fine; a struct that
// DOES carry one must be detected by the same ratchet.
func TestStateMarshalRefusesSecretField(t *testing.T) {
	s := &State{
		UUIDs:       map[string]string{"app/web/staging": "hzw53gga4fcgpsl706h5rgmo"},
		ResolvedAt:  time.Unix(0, 0).UTC(),
		OpenAPIHash: "deadbeef",
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("clean State must marshal: %v", err)
	}
	if !strings.Contains(string(out), "hzw53gga4fcgpsl706h5rgmo") {
		t.Errorf("marshalled State missing UUID: %s", out)
	}

	type leakyState struct {
		Token secrets.Secret `json:"token"`
	}
	if !containsSecret(reflect.TypeOf(leakyState{})) {
		t.Error("ratchet failed: a struct with a Secret field was NOT detected")
	}
	if containsSecret(reflect.TypeOf(State{})) {
		t.Error("ratchet false positive: clean State flagged as containing a Secret")
	}

	// Nested / pointer / slice reach.
	type nested struct {
		Inner *leakyState
	}
	if !containsSecret(reflect.TypeOf(nested{})) {
		t.Error("ratchet failed to detect Secret behind a pointer")
	}
}
