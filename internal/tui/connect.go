package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// ConnectInput is the raw credential set the wizard collects. Token and CFAccessSecret are
// plain strings only in transit: the injected connector turns them into ${env:} secrets (via
// os.Setenv + secrets.NewFromEnv), never a hardcoded Secret literal, and the wizard masks both
// on screen.
type ConnectInput struct {
	URL            string
	Token          string
	CFAccessID     string
	CFAccessSecret string
}

// ConnectFunc builds and tests a client from wizard input. The command injects it so the wizard
// never imports cmd: the closure owns the os.Setenv + buildClient + connection test, returning a
// ready client or a redacted error. It is never expected to return a nil client with a nil error.
type ConnectFunc func(ConnectInput) (*coolify.Client, error)

const (
	fieldURL = iota
	fieldToken
	fieldCFID
	fieldCFSecret
	numConnectFields
)

// connectedMsg carries the connector's outcome back to the wizard loop. The connection test
// runs in a tea.Cmd, so the loop never blocks on the network.
type connectedMsg struct {
	client *coolify.Client
	err    error
}

// ConnectModel is the credential wizard shown when explore starts with no credentials in the
// environment. It collects a URL, an API token (masked), and an optional CF-Access pair (the
// secret masked), tests the connection, and exposes the connected client once the test passes.
type ConnectModel struct {
	inputs     []textinput.Model
	focus      int
	connect    ConnectFunc
	connecting bool
	client     *coolify.Client // set once the connection test passes
	err        error
}

// NewConnectModel returns a wizard bound to connect, the closure that builds and tests a client
// from the entered values.
func NewConnectModel(connect ConnectFunc) ConnectModel {
	inputs := make([]textinput.Model, numConnectFields)
	for i := range inputs {
		inputs[i] = textinput.New()
	}
	inputs[fieldURL].Placeholder = "https://coolify.example.com"
	inputs[fieldToken].EchoMode = textinput.EchoPassword
	inputs[fieldCFSecret].EchoMode = textinput.EchoPassword
	inputs[fieldURL].Focus()
	return ConnectModel{inputs: inputs, connect: connect}
}

// Client returns the connected client once the wizard has tested a connection, or nil if the
// user quit first. The command uses it to decide whether to open the browser.
func (m ConnectModel) Client() *coolify.Client { return m.client }

// Init starts the cursor blink on the focused field.
func (m ConnectModel) Init() tea.Cmd { return textinput.Blink }

// Update advances the wizard. The connection test arrives as connectedMsg; key presses navigate,
// submit, or quit; any other message is forwarded to the focused input.
func (m ConnectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case connectedMsg:
		m.connecting = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.client = msg.client
		return m, tea.Quit
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m.updateFocused(msg)
}

// handleKey routes a key press. While a connection test is in flight every key is swallowed so a
// stray press can neither resubmit nor quit mid-test.
func (m ConnectModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.connecting {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		return m.submit()
	case "tab", "down":
		m.focus = (m.focus + 1) % len(m.inputs)
		return m, m.refocus()
	case "shift+tab", "up":
		m.focus = (m.focus - 1 + len(m.inputs)) % len(m.inputs)
		return m, m.refocus()
	}
	return m.updateFocused(msg)
}

// submit validates the form and, when valid, launches the connection test off the update loop.
// A validation error stays on the form with the message shown.
func (m ConnectModel) submit() (tea.Model, tea.Cmd) {
	in := m.input()
	if err := validateConnect(in); err != nil {
		m.err = err
		return m, nil
	}
	m.err = nil
	m.connecting = true
	return m, connectCmd(m.connect, in)
}

// input reads the entered values. URL and the CF-Access ID are trimmed; the token and CF-Access
// secret are passed verbatim so a credential with leading/trailing characters is not corrupted.
func (m ConnectModel) input() ConnectInput {
	return ConnectInput{
		URL:            strings.TrimSpace(m.inputs[fieldURL].Value()),
		Token:          m.inputs[fieldToken].Value(),
		CFAccessID:     strings.TrimSpace(m.inputs[fieldCFID].Value()),
		CFAccessSecret: m.inputs[fieldCFSecret].Value(),
	}
}

// refocus focuses the current field and blurs the rest, returning the focused input's blink.
func (m *ConnectModel) refocus() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.inputs {
		if i == m.focus {
			cmd = m.inputs[i].Focus()
			continue
		}
		m.inputs[i].Blur()
	}
	return cmd
}

// updateFocused forwards a message to the focused input only. inputs is a slice, so the value
// receiver mutates the shared backing array in place.
func (m ConnectModel) updateFocused(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

// validateConnect mirrors NewClient's contract at the form boundary: URL and token are
// required, and CF-Access is all-or-nothing.
func validateConnect(in ConnectInput) error {
	if in.URL == "" || in.Token == "" {
		return fmt.Errorf("a Coolify URL and API token are both required")
	}
	if (in.CFAccessID != "") != (in.CFAccessSecret != "") {
		return fmt.Errorf("CF-Access requires both Client ID and Client Secret, or neither")
	}
	return nil
}

// connectCmd runs the injected connector off the update loop.
func connectCmd(connect ConnectFunc, in ConnectInput) tea.Cmd {
	return func() tea.Msg {
		client, err := connect(in)
		return connectedMsg{client: client, err: err}
	}
}

// View renders the form. The token and CF-Access secret fields echo a mask, so no entered
// credential character is ever drawn.
func (m ConnectModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("connect to Coolify"))
	b.WriteByte('\n')
	b.WriteString(dimStyle.Render("no credentials in the environment — enter them to browse the live instance"))
	b.WriteString("\n\n")

	labels := []string{"Coolify URL", "API token", "CF-Access Client ID", "CF-Access Client Secret"}
	hints := []string{"", "", "(optional)", "(optional)"}
	for i := range m.inputs {
		cursor := "  "
		if i == m.focus {
			cursor = cursorStyle.Render("> ")
		}
		fmt.Fprintf(&b, "%s%-24s %s %s\n", cursor, labels[i], m.inputs[i].View(), dimStyle.Render(hints[i]))
	}

	b.WriteByte('\n')
	if m.connecting {
		b.WriteString(statusStyle.Render("connecting…"))
	} else {
		b.WriteString(dimStyle.Render("tab/↑↓ move · enter connect · esc quit"))
	}
	if m.err != nil {
		b.WriteByte('\n')
		b.WriteString(errStyle.Render("error: " + m.err.Error()))
	}
	return b.String()
}
