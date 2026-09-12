package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

func (m Model) renderTabs() string {
	if len(m.cfg.Servers) == 0 {
		return ui.Truncate("sing-box-tui · Local setup", m.width)
	}
	labels := []string{"groups", "connections", "logs"}
	var parts []string
	for i, label := range labels {
		text := fmt.Sprintf("%d %s", i+1, label)
		if Page(i) == m.page {
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
		next = "r retry    e edit server    s servers"
	}
	lines = append(lines, ui.DimStyle.Render(next))
	return strings.Join(lines, "\n")
}

func (m Model) helpLines() []string {
	rows := [][2]string{
		{"1 2 3", "pages"},
		{"j k / ↑ ↓", "move"},
		{"Tab / S-Tab", "next / previous page"},
		{"PgUp/PgDn", "page up / down"},
		{"Ctrl+u/d", "half page up / down"},
		{"Home / End", "first / last (gg / G)"},
		{"F2 / !", "read full error"},
		{"enter", "select"},
		{"[ ]", "clash mode"},
		{"s", "servers"},
		{"r", "reconnect"},
		{"?", "help"},
		{"q", "quit"},
	}
	switch {
	case len(m.cfg.Servers) == 0 || m.returnMode == modeServers:
		rows = append(rows, [2]string{"", ""}, [2]string{"a", "add server"}, [2]string{"d", "delete server"}, [2]string{"e", "edit server"}, [2]string{"r", "scan localhost (setup)"})
	case m.page == PageGroups:
		rows = append(rows, [2]string{"", ""}, [2]string{"t", "url test"}, [2]string{"e", "expand"}, [2]string{"v", "full names / details"})
	case m.page == PageConnections:
		rows = append(rows,
			[2]string{"", ""},
			[2]string{"/", "search"},
			[2]string{"enter", "full connection details"},
			[2]string{"p", "pause / resume refresh"},
			[2]string{"f", "active / all / closed"},
			[2]string{"x", "close connection"},
			[2]string{"D", "close all"},
		)
	case m.page == PageLogs:
		rows = append(rows, [2]string{"", ""}, [2]string{"f", "level"}, [2]string{"c", "clear"}, [2]string{"enter", "read log snapshot"}, [2]string{"G", "resume following logs"})
	}
	if m.usingClash() {
		rows = append(rows, [2]string{"", ""}, [2]string{"clash", "Selector only · no uptime · c is local"})
	}
	rows = append(rows, [2]string{"", ""}, [2]string{"Details", "j/k scroll; w wrap; h/l pan; y copy"}, [2]string{"esc / ?", "close"})

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
	return strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, m.width), true), "\n")
}

func (m Model) viewHelp(height int) string {
	lines := m.helpLines()
	h := max(1, height-1)
	top := clamp(m.helpOffset, 0, max(0, len(lines)-h))
	end := min(len(lines), top+h)
	return strings.Join(append([]string{ui.Truncate(fmt.Sprintf("Help · %d–%d/%d · ↑↓ scroll", top+1, end, len(lines)), m.width)}, lines[top:end]...), "\n")
}

func (m Model) confirmLines() []string {
	var question string
	switch m.confirm {
	case confirmCloseOne:
		question = "Close connection to " + m.closeTarget + "?\nID: " + m.closeID
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
		ui.DimStyle.Render("y confirm    esc cancel"),
	}
	return strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), max(1, m.width), true), "\n")
}

func (m Model) renderFooter() string {

	hint := "? help · q quit"
	switch m.mode {
	case modeDetail:
		hint = "w wrap · y copy · ↑↓ scroll · esc back"
	case modeHelp:
		hint = "↑↓ scroll · PgUp/PgDn page · esc back"
	case modeConfirm:
		hint = "y confirm · esc cancel"
	case modeServerForm:
		hint = "Esc back · Enter save · Tab/↑↓ · ^t type"
	case modeServers:
		hint = "? help · e edit · a add · Enter use · d delete"
	case modeSearch:
		hint = "Enter filter · Esc clear"
	default:
		if len(m.cfg.Servers) == 0 {
			hint = "a add · r scan · Enter use · ? help"
		} else if !m.connected() {
			hint = "r retry · e edit · s servers · ? help"
		} else {
			switch m.page {
			case PageGroups:
				hint = "? help · Enter select · e expand · t test · v details"
			case PageConnections:
				hint = "? help · Enter details · / search · p pause · x close"
			case PageLogs:
				hint = "? help · Enter read · f level · G follow · c clear"
			}
		}
	}
	return ui.Truncate(hint, m.width)
}

func (m Model) renderMessage() string {
	if m.statusError {
		return ui.Truncate("F2 details · "+ui.ErrorStyle.Render(m.statusMsg), m.width)
	}
	return ui.Truncate(m.statusMsg, m.width)
}

func (m Model) viewConfirm(height int) string {
	lines := m.confirmLines()
	h := max(1, height-1)
	top := clamp(m.confirmOffset, 0, max(0, len(lines)-h))
	end := min(len(lines), top+h)
	title := "Confirm action"
	if len(lines) > h {
		title += " · ↑↓ scroll"
	}
	return strings.Join(append([]string{ui.Truncate(title, m.width)}, lines[top:end]...), "\n")
}
