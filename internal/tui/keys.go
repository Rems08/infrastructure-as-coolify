package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap is the browser's key bindings, also feeding the help bar.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Open   key.Binding
	Back   key.Binding
	Reveal key.Binding
	Logs   key.Binding
	Quit   key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open")),
		Back:   key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
		Reveal: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reveal")),
		Logs:   key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "logs")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Reveal, k.Logs, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Open, k.Back}, {k.Reveal, k.Logs, k.Quit}}
}
