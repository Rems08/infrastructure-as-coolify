package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

func connectStep(t *testing.T, m ConnectModel, msg tea.Msg) (ConnectModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	cm, ok := next.(ConnectModel)
	if !ok {
		t.Fatalf("Update returned %T, want ConnectModel", next)
	}
	return cm, cmd
}

func enterKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

// okConnector returns a connector that always hands back a ready client, so the wizard's
// success path can be exercised without a network.
func okConnector(t *testing.T) ConnectFunc {
	t.Helper()
	client, err := coolify.NewClient(coolify.Options{BaseURL: "https://x", Token: secrets.NewRemote("t")})
	if err != nil {
		t.Fatal(err)
	}
	return func(ConnectInput) (*coolify.Client, error) { return client, nil }
}

func TestConnect_TokenAndCFSecretAreMasked(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m.inputs[fieldToken].SetValue("super-token-DO_NOT_SHOW")
	m.inputs[fieldCFSecret].SetValue("super-cf-DO_NOT_SHOW")
	m.inputs[fieldURL].SetValue("https://coolify.example")
	m.inputs[fieldCFID].SetValue("cf-id-visible")

	view := m.View()
	for _, secret := range []string{"super-token-DO_NOT_SHOW", "super-cf-DO_NOT_SHOW"} {
		if strings.Contains(view, secret) {
			t.Errorf("masked field leaked %q into the view:\n%s", secret, view)
		}
	}
	// The URL and CF-Access ID are not secrets and stay visible.
	if !strings.Contains(view, "https://coolify.example") {
		t.Errorf("URL field must render in clear:\n%s", view)
	}
}

func TestConnect_SubmitRequiresURLAndToken(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m.inputs[fieldURL].SetValue("https://coolify.example") // token left empty

	m, cmd := connectStep(t, m, enterKey())
	if m.err == nil || !strings.Contains(m.err.Error(), "required") {
		t.Fatalf("submit without a token must show a validation error, got %v", m.err)
	}
	if m.connecting || cmd != nil {
		t.Error("an invalid form must not launch the connection test")
	}
}

func TestConnect_CFAccessIsAllOrNothing(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m.inputs[fieldURL].SetValue("https://coolify.example")
	m.inputs[fieldToken].SetValue("tok")
	m.inputs[fieldCFID].SetValue("cf-id") // secret left empty

	m, cmd := connectStep(t, m, enterKey())
	if m.err == nil || !strings.Contains(m.err.Error(), "both") {
		t.Fatalf("a lone CF-Access ID must be rejected, got %v", m.err)
	}
	if m.connecting || cmd != nil {
		t.Error("a both-or-neither violation must not launch the connection test")
	}
}

func TestConnect_SuccessExposesClientAndQuits(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m.inputs[fieldURL].SetValue("https://coolify.example")
	m.inputs[fieldToken].SetValue("tok")

	m, cmd := connectStep(t, m, enterKey())
	if !m.connecting || cmd == nil {
		t.Fatal("a valid submit must launch the connection test off the loop")
	}
	done, ok := cmd().(connectedMsg)
	if !ok || done.err != nil || done.client == nil {
		t.Fatalf("connector message = %+v, want a successful connectedMsg", done)
	}

	m, cmd = connectStep(t, m, done)
	if m.Client() == nil {
		t.Error("a successful connection must expose the client")
	}
	if m.connecting {
		t.Error("connecting must clear once the test resolves")
	}
	if cmd == nil {
		t.Error("a successful connection must quit the wizard")
	}
}

func TestConnect_FailureStaysOpenRedactedThenRetries(t *testing.T) {
	calls := 0
	connect := func(ConnectInput) (*coolify.Client, error) {
		calls++
		if calls == 1 {
			return nil, errConnRefused
		}
		client, _ := coolify.NewClient(coolify.Options{BaseURL: "https://x", Token: secrets.NewRemote("t")})
		return client, nil
	}
	m := NewConnectModel(connect)
	m.inputs[fieldURL].SetValue("https://coolify.example")
	m.inputs[fieldToken].SetValue("super-token-DO_NOT_SHOW")

	m, cmd := connectStep(t, m, enterKey())
	m, _ = connectStep(t, m, cmd().(connectedMsg))
	if m.err == nil {
		t.Fatal("a failed connection must surface an error and stay open")
	}
	if strings.Contains(m.err.Error(), "super-token-DO_NOT_SHOW") {
		t.Errorf("connection error leaked the token: %v", m.err)
	}
	if m.Client() != nil {
		t.Error("a failed connection must not expose a client")
	}
	if strings.Contains(m.View(), "[S]") { // still the form, not the browser menu
		t.Error("a failed connection must keep the wizard open")
	}

	// Retry: a second submit re-runs the connector, which now succeeds.
	m, cmd = connectStep(t, m, enterKey())
	m, _ = connectStep(t, m, cmd().(connectedMsg))
	if m.Client() == nil {
		t.Error("a retry after a fixed credential must connect")
	}
}

func TestConnect_NavigationMovesFocusAndTypes(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	if m.Init() == nil {
		t.Error("Init must start the cursor blink")
	}
	if m.focus != fieldURL {
		t.Fatalf("focus starts at %d, want fieldURL", m.focus)
	}

	m, _ = connectStep(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != fieldToken {
		t.Errorf("tab from URL must focus the token field, got %d", m.focus)
	}
	m, _ = connectStep(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != fieldURL {
		t.Errorf("shift+tab must move focus back to URL, got %d", m.focus)
	}
	// shift+tab wraps to the last field.
	m, _ = connectStep(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != fieldCFSecret {
		t.Errorf("shift+tab past the first field must wrap to the last, got %d", m.focus)
	}

	// A printable key reaches the focused input.
	m, _ = connectStep(t, m, keyRunes('h'))
	if m.inputs[fieldCFSecret].Value() != "h" {
		t.Errorf("typed rune did not reach the focused input: %q", m.inputs[fieldCFSecret].Value())
	}
}

func TestConnect_KeysSwallowedWhileConnecting(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m.connecting = true
	m, cmd := connectStep(t, m, enterKey())
	if cmd != nil {
		t.Error("keys must be swallowed while a connection test is in flight")
	}
	if !m.connecting {
		t.Error("an in-flight test must stay in the connecting state")
	}
}

func TestConnect_EscQuitsWithoutClient(t *testing.T) {
	m := NewConnectModel(okConnector(t))
	m, cmd := connectStep(t, m, escKey())
	if cmd == nil {
		t.Error("esc must quit the wizard")
	}
	if m.Client() != nil {
		t.Error("quitting before connecting must leave the client nil")
	}
}

var errConnRefused = errConn("dial tcp: connection refused")

type errConn string

func (e errConn) Error() string { return string(e) }
