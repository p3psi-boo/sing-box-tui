package app

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keyMap struct {
	Groups    key.Binding
	Conns     key.Binding
	Logs      key.Binding
	NextTab   key.Binding
	PrevTab   key.Binding
	Quit      key.Binding
	Help      key.Binding
	Reconnect key.Binding
	Servers   key.Binding
	ModePrev  key.Binding
	ModeNext  key.Binding
	Up        key.Binding
	Down      key.Binding
	Top       key.Binding
	Bottom    key.Binding
	PageDown  key.Binding
	PageUp    key.Binding
	HalfDown  key.Binding
	HalfUp    key.Binding
	Confirm   key.Binding
	Back      key.Binding
	Search    key.Binding
	Filter    key.Binding
	Test      key.Binding
	Expand    key.Binding
	CloseOne  key.Binding
	CloseAll  key.Binding
	Add       key.Binding
	Delete    key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Groups:    key.NewBinding(key.WithKeys("1")),
		Conns:     key.NewBinding(key.WithKeys("2")),
		Logs:      key.NewBinding(key.WithKeys("3")),
		NextTab:   key.NewBinding(key.WithKeys("tab")),
		PrevTab:   key.NewBinding(key.WithKeys("shift+tab")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c")),
		Help:      key.NewBinding(key.WithKeys("?")),
		Reconnect: key.NewBinding(key.WithKeys("r")),
		Servers:   key.NewBinding(key.WithKeys("s")),
		ModePrev:  key.NewBinding(key.WithKeys("[")),
		ModeNext:  key.NewBinding(key.WithKeys("]")),
		Up:        key.NewBinding(key.WithKeys("k", "up")),
		Down:      key.NewBinding(key.WithKeys("j", "down")),
		Top:       key.NewBinding(key.WithKeys("home")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end")),
		PageDown:  key.NewBinding(key.WithKeys("ctrl+f", "pgdown")),
		PageUp:    key.NewBinding(key.WithKeys("ctrl+b", "pgup")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u")),
		Confirm:   key.NewBinding(key.WithKeys("enter")),
		Back:      key.NewBinding(key.WithKeys("esc")),
		Search:    key.NewBinding(key.WithKeys("/")),
		Filter:    key.NewBinding(key.WithKeys("f")),
		Test:      key.NewBinding(key.WithKeys("t")),
		Expand:    key.NewBinding(key.WithKeys("e")),
		CloseOne:  key.NewBinding(key.WithKeys("x")),
		CloseAll:  key.NewBinding(key.WithKeys("D")),
		Add:       key.NewBinding(key.WithKeys("a")),
		Delete:    key.NewBinding(key.WithKeys("d")),
	}
}

type inputMode int

const (
	modeNormal inputMode = iota
	modeSearch
	modeHelp
	modeServers
	modeServerForm
	modeConfirm
)

func (m *Model) resolveKey(msg tea.KeyMsg) string {
	k := msg.String()
	if m.pendingKey == "g" {
		m.pendingKey = ""
		if k == "g" {
			return "gg"
		}
	}
	if k == "g" {
		m.pendingKey = "g"
		m.pendingKeyAt = time.Now()
		return ""
	}
	m.pendingKey = ""
	return k
}

func (m *Model) expirePendingKey() {
	if m.pendingKey != "" && time.Since(m.pendingKeyAt) > 800*time.Millisecond {
		m.pendingKey = ""
	}
}

func (m *Model) isUp(msg tea.KeyMsg) bool   { return key.Matches(msg, m.keys.Up) }
func (m *Model) isDown(msg tea.KeyMsg) bool { return key.Matches(msg, m.keys.Down) }
