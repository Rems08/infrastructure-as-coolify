package plan_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

var update = flag.Bool("update", false, "update golden files")

func res(kind, name string, fields ...plan.Field) plan.Resource {
	return plan.Resource{Kind: kind, Name: name, Fields: fields}
}

func f(name string, v plan.Value) plan.Field { return plan.Field{Name: name, Value: v} }

func secretEnv(t *testing.T, env, val string) secrets.Secret {
	t.Helper()
	t.Setenv(env, val)
	s, err := secrets.NewFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDiffCreate(t *testing.T) {
	desired := res("Application", "web",
		f("fqdn", plan.Scalar("https://web.example.com")),
		f("port", plan.Scalar("3000")),
	)
	changes := plan.Diff(desired, nil)
	if len(changes) != 2 {
		t.Fatalf("create diff = %d changes, want 2: %+v", len(changes), changes)
	}
	for _, c := range changes {
		if c.Op != plan.OpAdd {
			t.Errorf("create change op = %s, want add", c.Op)
		}
	}
}

func TestDiffScalarCases(t *testing.T) {
	desired := res("Application", "web",
		f("fqdn", plan.Scalar("https://new.example.com")), // updated
		f("port", plan.Scalar("3000")),                    // unchanged
		f("tag", plan.Scalar("v2")),                       // added (missing in actual)
	)
	actual := res("Application", "web",
		f("fqdn", plan.Scalar("https://old.example.com")),
		f("port", plan.Scalar("3000")),
		f("legacy", plan.Scalar("x")), // deleted (absent in desired)
	)
	changes := plan.Diff(desired, &actual)

	got := map[string]plan.Change{}
	for _, c := range changes {
		got[c.Path] = c
	}
	if c := got["Application.web.fqdn"]; c.Op != plan.OpUpdate || c.Old != "https://old.example.com" || c.New != "https://new.example.com" {
		t.Errorf("fqdn change = %+v", c)
	}
	if _, ok := got["Application.web.port"]; ok {
		t.Error("unchanged port must not appear")
	}
	if c := got["Application.web.tag"]; c.Op != plan.OpAdd || c.New != "v2" {
		t.Errorf("tag change = %+v", c)
	}
	if c := got["Application.web.legacy"]; c.Op != plan.OpDelete || c.Old != "x" {
		t.Errorf("legacy change = %+v", c)
	}
}

func TestDiffNoop(t *testing.T) {
	r := res("Application", "web", f("fqdn", plan.Scalar("https://x")))
	if changes := plan.Diff(r, &r); len(changes) != 0 {
		t.Errorf("identical resources must produce 0 changes, got %+v", changes)
	}
}

// TestSecretDiffNotifyOnlyNeverLeaksValue asserts a secret value change is announced
// without the value, hash, or any partial appearing in the diff output.
func TestSecretDiffNotifyOnlyNeverLeaksValue(t *testing.T) {
	const secretVal = "super-secret-VALUE-do-not-leak"

	t.Run("same source same value is noop", func(t *testing.T) {
		s := secretEnv(t, "TOK", secretVal)
		r := res("Application", "web", f("env.TOK", plan.SecretValue(s)))
		if changes := plan.Diff(r, &r); len(changes) != 0 {
			t.Errorf("identical secret must be noop, got %+v", changes)
		}
	})

	t.Run("same source changed value notifies without leaking", func(t *testing.T) {
		old := secretEnv(t, "TOK", "old-"+secretVal)
		newv := secretEnv(t, "TOK", "new-"+secretVal)
		desired := res("Application", "web", f("env.TOK", plan.SecretValue(newv)))
		actual := res("Application", "web", f("env.TOK", plan.SecretValue(old)))
		changes := plan.Diff(desired, &actual)
		if len(changes) != 1 {
			t.Fatalf("want 1 change, got %+v", changes)
		}
		c := changes[0]
		if !c.Sensitive {
			t.Error("secret change must be marked sensitive")
		}
		if !strings.Contains(c.New, "resolved value changed") || !strings.Contains(c.New, "${env:TOK}") {
			t.Errorf("note = %q, want notify-only message", c.New)
		}
		assertNoLeak(t, c, secretVal)
	})

	t.Run("changed source shows only origins", func(t *testing.T) {
		old := secretEnv(t, "OLDTOK", secretVal)
		newv := secretEnv(t, "NEWTOK", secretVal)
		desired := res("Application", "web", f("env.TOK", plan.SecretValue(newv)))
		actual := res("Application", "web", f("env.TOK", plan.SecretValue(old)))
		changes := plan.Diff(desired, &actual)
		if len(changes) != 1 {
			t.Fatalf("want 1 change, got %+v", changes)
		}
		c := changes[0]
		if c.Old != "${env:OLDTOK}" || c.New != "${env:NEWTOK}" {
			t.Errorf("origin change = %q -> %q", c.Old, c.New)
		}
		assertNoLeak(t, c, secretVal)
	})

	t.Run("rendered text never leaks", func(t *testing.T) {
		old := secretEnv(t, "TOK", "old-"+secretVal)
		newv := secretEnv(t, "TOK", "new-"+secretVal)
		var p plan.Plan
		p.Add(res("Application", "web", f("env.TOK", plan.SecretValue(newv))),
			ptr(res("Application", "web", f("env.TOK", plan.SecretValue(old)))))
		text := p.RenderText()
		if strings.Contains(text, secretVal) {
			t.Errorf("rendered plan leaks secret value:\n%s", text)
		}
		blob, _ := json.Marshal(p.Output())
		if strings.Contains(string(blob), secretVal) {
			t.Errorf("json plan leaks secret value:\n%s", blob)
		}
	})
}

func TestSecretToScalarFlip(t *testing.T) {
	s := secretEnv(t, "TOK", "v")
	desired := res("Application", "web", f("x", plan.Scalar("plain")))
	actual := res("Application", "web", f("x", plan.SecretValue(s)))
	changes := plan.Diff(desired, &actual)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate || !changes[0].Sensitive {
		t.Fatalf("flip change = %+v", changes)
	}
}

// TestSecretUnresolvedDesiredNoFalsePositive guards the read-only flow: a desired secret
// loaded without its value (its ${env:} reference is bound only at apply) must not diff as a
// phantom "resolved value changed" against a remote value when the source is unchanged.
func TestSecretUnresolvedDesiredNoFalsePositive(t *testing.T) {
	old := secretEnv(t, "DBURL_X", "postgres://live")
	newv, err := secrets.NewReference("${env:DBURL_X}")
	if err != nil {
		t.Fatal(err)
	}
	desired := res("Application", "web", f("env.DATABASE_URL", plan.SecretValue(newv)))
	actual := res("Application", "web", f("env.DATABASE_URL", plan.SecretValue(old)))
	if changes := plan.Diff(desired, &actual); len(changes) != 0 {
		t.Errorf("Diff = %+v, want no change for an unresolved desired secret", changes)
	}
}

// TestSecretDifferentOriginStillUpdates asserts a genuinely changed source is still reported
// even when the desired secret is unresolved, and without leaking any value.
func TestSecretDifferentOriginStillUpdates(t *testing.T) {
	old := secretEnv(t, "OLD_REF", "live-value")
	newv, err := secrets.NewReference("${env:NEW_REF}")
	if err != nil {
		t.Fatal(err)
	}
	desired := res("Application", "web", f("env.X", plan.SecretValue(newv)))
	actual := res("Application", "web", f("env.X", plan.SecretValue(old)))
	changes := plan.Diff(desired, &actual)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate {
		t.Fatalf("Diff = %+v, want one Update on a differing origin", changes)
	}
	assertNoLeak(t, changes[0], "live-value")
}

func assertNoLeak(t *testing.T, c plan.Change, secret string) {
	t.Helper()
	if strings.Contains(c.Old, secret) || strings.Contains(c.New, secret) {
		t.Errorf("change leaks secret value: %+v", c)
	}
}

func ptr(r plan.Resource) *plan.Resource { return &r }

func TestPlanSummaryAndExit(t *testing.T) {
	var p plan.Plan
	p.Add(res("Application", "new", f("fqdn", plan.Scalar("https://a"))), nil) // create
	updActual := res("Application", "chg", f("fqdn", plan.Scalar("https://old")))
	p.Add(res("Application", "chg", f("fqdn", plan.Scalar("https://new"))), &updActual) // update
	noop := res("Application", "same", f("fqdn", plan.Scalar("https://s")))
	p.Add(noop, &noop) // noop

	s := p.Summary()
	if s.Add != 1 || s.Change != 1 || s.Destroy != 0 {
		t.Errorf("summary = %+v, want add1 change1 destroy0", s)
	}
	if !p.HasChanges() {
		t.Error("plan with create+update must report HasChanges")
	}

	var empty plan.Plan
	empty.Add(noop, &noop)
	if empty.HasChanges() {
		t.Error("noop-only plan must not report changes")
	}
}

func TestRenderTextGolden(t *testing.T) {
	var p plan.Plan
	p.Add(res("Application", "new",
		f("fqdn", plan.Scalar("https://new.example.com")),
		f("port", plan.Scalar("8080")),
	), nil)
	updActual := res("Application", "web", f("fqdn", plan.Scalar("https://old")), f("port", plan.Scalar("3000")))
	p.Add(res("Application", "web",
		f("fqdn", plan.Scalar("https://web.example.com")),
		f("port", plan.Scalar("3000")),
	), &updActual)
	movedActual := res("Application", "moved",
		plan.Field{Name: "destination.server", Value: plan.Scalar("localhost"), ForcesRecreate: true},
	)
	p.Add(res("Application", "moved",
		plan.Field{Name: "destination.server", Value: plan.Scalar("hetzner-1"), ForcesRecreate: true},
	), &movedActual)

	got := p.RenderText()
	goldenPath := filepath.Join("testdata", "plan_render.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("rendered plan differs from golden; run `go test ./internal/plan -update`\n--- got ---\n%s", got)
	}
}

func TestValueDisplayRedactedWhenNoOrigin(t *testing.T) {
	// A zero secret has no origin; an addition must fall back to [REDACTED], never panic.
	desired := res("Application", "web", f("x", plan.SecretValue(secrets.Secret{})))
	changes := plan.Diff(desired, nil)
	if len(changes) != 1 || changes[0].New != "[REDACTED]" {
		t.Fatalf("zero-secret add = %+v", changes)
	}
}
