package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

type discoveryState struct {
	scanning bool
	results  []client.DiscoveredServer
	cursor   int
	err      error
}
type discoveryResultMsg struct {
	results []client.DiscoveredServer
	err     error
}

func discoverCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		results, err := client.DiscoverLocal(ctx)
		return discoveryResultMsg{results: results, err: err}
	}
}

func (m Model) viewSetup() string {
	title := "No servers yet · localhost discovery"
	message := "No API found. Enable a sing-box management API, then press r to scan again."
	if m.discovery.scanning {
		message = "Scanning localhost (IPv4 / IPv6)… You can press a to add manually."
	}
	if m.discovery.err != nil {
		message = "Scan timed out; results may be incomplete. Press r to retry or a to add manually."
	}
	if len(m.discovery.results) > 0 && m.discovery.err == nil {
		message = "Enter review · ? unverified; needs secret"
	}
	if len(m.discovery.results) == 0 {
		return ui.Truncate(title, m.width) + "\n\n" + wrapText(message, m.width)
	}
	h := max(1, m.contentHeight()-2)
	start, end, _ := visibleWindow(len(m.discovery.results), m.discovery.cursor, 0, h)
	lines := []string{ui.Truncate(fmt.Sprintf("Local APIs · %d–%d/%d", start+1, end, len(m.discovery.results)), m.width), ui.Truncate(message, m.width)}
	for i := start; i < end; i++ {
		r := m.discovery.results[i]
		label := r.API + " " + r.Version
		if !r.Verified {
			label = "? " + r.API + " · secret required"
		}
		line := ui.Truncate(cursorPrefix(i == m.discovery.cursor)+r.Address+"  "+label, m.width)
		if i == m.discovery.cursor {
			line = ui.SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) useDiscovered() {
	if len(m.discovery.results) == 0 {
		return
	}
	r := m.discovery.results[clamp(m.discovery.cursor, 0, len(m.discovery.results)-1)]
	m.openForm()
	m.form.rebuild(map[string]string{"name": "Local " + r.API, "address": r.Address, "api": r.API})
	if r.NeedsSecret {
		for i, label := range m.form.labels() {
			if label == "secret" {
				m.form.focus = i
			}
		}
	}
	m.form.applyFocus()
	m.resizeForm()
}
