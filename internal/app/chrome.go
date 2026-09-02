package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderTabs() string {
	labels := []string{"groups", "connections", "logs"}
	var parts []string
	for i, label := range labels {
		text := fmt.Sprintf("%d %s", i+1, label)
		if Page(i) == m.page && m.mode == modeNormal {
			parts = append(parts, ui.TabActive.Render(text))
		} else {
			parts = append(parts, ui.TabInactive.Render(text))
		}
	}
	line := strings.Join(parts, "  ")
	return ui.Truncate(line, m.width)
}

func (m Model) renderStatusBar() string {
	var parts []string
	server := m.cfg.ActiveServer()
	name := "none"
	if server != nil {
		name = server.DisplayName()
	}

	dot := ui.StatusOK.Render("●")
	if m.connected() {
		parts = append(parts, dot+" "+name)
		switch m.snapshot.ServiceStatus {
		case daemon.ServiceStatus_STARTING:
			parts = append(parts, "starting")
		case daemon.ServiceStatus_STOPPING:
			parts = append(parts, "stopping")
		case daemon.ServiceStatus_FATAL:
			parts = append(parts, ui.StatusFail.Render("fatal"))
		case daemon.ServiceStatus_IDLE:
			parts = append(parts, "idle")
		}
		mode := m.snapshot.ClashMode
		if m.pendingMode != "" {
			mode = m.pendingMode
		}
		if mode != "" {
			parts = append(parts, mode)
		}
		if st := m.snapshot.Status; st != nil {
			if st.TrafficAvailable {
				parts = append(parts, "↑"+ui.FormatBitrateShort(st.Uplink)+" ↓"+ui.FormatBitrateShort(st.Downlink))
			}
			if st.ConnectionsIn > 0 {
				parts = append(parts, fmt.Sprintf("%d in %d out", st.ConnectionsIn, st.ConnectionsOut))
			} else if st.ConnectionsOut > 0 {
				parts = append(parts, fmt.Sprintf("%d conn", st.ConnectionsOut))
			}
		}
		if !m.snapshot.StartedAt.IsZero() {
			parts = append(parts, ui.FormatUptimeShort(m.snapshot.StartedAt))
		}
		if m.usingClash() {
			parts = append(parts, ui.StatusMuted.Render("clash"))
		}
	} else if server != nil {
		parts = append(parts, ui.StatusFail.Render("○")+" "+name)
		parts = append(parts, "unreachable")
	} else {
		parts = append(parts, "no server")
	}

	switch {
	case m.mode == modeSearch || (m.page == PageConnections && m.connSearch.Value() != "" && m.mode == modeNormal):
		parts = append(parts, "/"+m.connSearch.Value())
	case m.page == PageConnections && m.mode == modeNormal && m.connected():
		parts = append(parts, fmt.Sprintf("%d %s", len(m.filteredConnections()), m.connFilter.String()))
	case m.page == PageLogs && m.mode == modeNormal && m.connected():
		follow := "follow"
		if !m.logFollow {
			follow = "paused"
		}
		parts = append(parts, strings.ToLower(ui.LogLevelLabel(m.logThreshold()))+"+ "+follow)
	case m.mode == modeServerForm:
		parts = append(parts, "enter save  ctrl+t type  esc cancel")
	case m.mode == modeServers:
		parts = append(parts, "enter use  a add  d delete  esc")
	}

	if m.statusMsg != "" && time.Since(m.statusAt) < statusTTL {
		parts = append(parts, m.statusMsg)
	}

	line := " " + strings.Join(parts, "  ")
	return ui.Truncate(line, m.width)
}

func (m Model) viewDisconnected(next string) string {
	server := m.cfg.ActiveServer()
	target := ui.ServerTarget(server)
	var lines []string
	if target != "" {
		lines = append(lines, "Can't reach "+target)
	} else {
		lines = append(lines, "Not connected")
	}
	if m.connectErr != "" {
		lines = append(lines, ui.DimStyle.Render(m.connectErr))
	} else if m.snapshot.ConnectError != "" {
		lines = append(lines, ui.DimStyle.Render(ui.CleanConnectError(m.snapshot.ConnectError)))
	}
	lines = append(lines, "")
	if next == "" {
		next = "r  retry    s  servers"
	}
	lines = append(lines, ui.DimStyle.Render(next))
	return strings.Join(lines, "\n")
}

func (m Model) viewHelp(height int) string {
	rows := [][2]string{
		{"1 2 3", "pages"},
		{"j k", "move"},
		{"enter", "select"},
		{"[ ]", "clash mode"},
		{"s", "servers"},
		{"r", "reconnect"},
		{"?", "help"},
		{"q", "quit"},
	}
	switch {
	case len(m.cfg.Servers) == 0 || m.returnMode == modeServers:
		rows = append(rows, [2]string{"", ""}, [2]string{"a", "add server"}, [2]string{"d", "delete server"})
	case m.page == PageGroups:
		rows = append(rows, [2]string{"", ""}, [2]string{"t", "url test"}, [2]string{"e", "expand"})
	case m.page == PageConnections:
		rows = append(rows,
			[2]string{"", ""},
			[2]string{"/", "search"},
			[2]string{"f", "active / all / closed"},
			[2]string{"x", "close connection"},
			[2]string{"D", "close all"},
		)
	case m.page == PageLogs:
		rows = append(rows, [2]string{"", ""}, [2]string{"f", "level"}, [2]string{"c", "clear"})
	}
	if m.usingClash() {
		rows = append(rows, [2]string{"", ""}, [2]string{"clash", "Selector only · no uptime · c is local"})
	}
	rows = append(rows, [2]string{"", ""}, [2]string{"esc / ?", "close"})

	keyWidth := 10
	var lines []string
	for _, row := range rows {
		if row[0] == "" && row[1] == "" {
			lines = append(lines, "")
			continue
		}
		key := fmt.Sprintf("%-*s", keyWidth, row[0])
		lines = append(lines, ui.DimStyle.Render(key)+" "+row[1])
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewConfirm(height int) string {
	var question string
	switch m.confirm {
	case confirmCloseAll:
		question = "Close all connections?"
	case confirmDeleteServer:
		name := "this server"
		if m.deleteIndex >= 0 && m.deleteIndex < len(m.cfg.Servers) {
			name = m.cfg.Servers[m.deleteIndex].DisplayName()
		}
		question = "Delete " + name + "?"
	default:
		question = "Confirm?"
	}
	lines := []string{
		question,
		"",
		ui.DimStyle.Render("y  confirm    esc  cancel"),
	}
	_ = height
	return strings.Join(lines, "\n")
}

func (m Model) viewSetup() string {
	lines := []string{
		"No servers yet",
		"",
		"Add a sing-box API to connect.",
		ui.DimStyle.Render("Saved to " + m.cfgPath),
		"",
		ui.DimStyle.Render("a  add server    q  quit"),
	}
	content := strings.Join(lines, "\n")
	if m.width > 0 && m.height > 0 {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(content)
	}
	return content
}
