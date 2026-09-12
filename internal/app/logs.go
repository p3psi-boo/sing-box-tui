package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

var logLevelCycle = []int32{2, 3, 4, 5, 6} // error → trace

func (m Model) logThreshold() int32 {
	if !m.logUseDefault && m.logLevelFilter >= 0 {
		return m.logLevelFilter
	}
	if m.snapshot.DefaultLogLevelSet {
		return int32(m.snapshot.DefaultLogLevel)
	}
	return 4
}

func filterLogs(entries []client.LogEntry, threshold int32) []client.LogEntry {
	out := make([]client.LogEntry, 0, len(entries))
	for _, e := range entries {
		if int32(e.Level) <= threshold {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) filteredLogs() []client.LogEntry {
	return filterLogs(m.snapshot.Logs, m.logThreshold())
}

func (m Model) viewLogs(height int) string {
	if !m.connected() {
		return m.viewDisconnected("")
	}
	follow := "follow"
	if !m.logFollow {
		follow = "paused"
	}
	head := fmt.Sprintf("%s+ · %s · f level · G follow", strings.ToLower(ui.LogLevelLabel(m.logThreshold())), follow)
	return ui.Truncate(head, m.width) + "\n" + m.viewLogEntries(max(1, height-1))
}

func (m Model) viewLogEntries(height int) string {
	if !m.connected() {
		return m.viewDisconnected("")
	}
	entries := m.filteredLogs()
	if len(entries) == 0 {
		if len(m.snapshot.Logs) > 0 {
			return ui.DimStyle.Render("No entries at this level")
		}
		return ui.DimStyle.Render("No log entries")
	}

	n := len(entries)
	var start int
	if m.logFollow {
		start = max(0, n-height)
	} else {
		start = clamp(m.logTop, 0, max(0, n-height))
		if start+height >= n {
			start = max(0, n-height)
		}
	}
	end := min(n, start+height)
	newer := 0
	if !m.logFollow && end < n && height > 1 {
		end = min(n, start+height-1)
		newer = n - end
	}

	var lines []string
	for _, entry := range entries[start:end] {
		lines = append(lines, renderLogLine(entry, m.width))
	}
	if newer > 0 {
		lines = append(lines, ui.DimStyle.Render(ui.Truncate(fmt.Sprintf("↓ %d newer", newer), m.width)))
	}
	return strings.Join(lines, "\n")
}

func renderLogLine(entry client.LogEntry, width int) string {
	label := ui.PadRight(strings.ToLower(ui.LogLevelLabel(int32(entry.Level))), 5)
	msg := entry.Message
	switch entry.Level {
	case daemon.LogLevel_PANIC, daemon.LogLevel_FATAL, daemon.LogLevel_ERROR:
		label = ui.ErrorStyle.Render(label)
	case daemon.LogLevel_WARN:
		label = ui.WarningStyle.Render(label)
	case daemon.LogLevel_DEBUG, daemon.LogLevel_TRACE:
		label = ui.DimStyle.Render(label)
		msg = ui.DimStyle.Render(msg)
	default:
		label = ui.DimStyle.Render(label)
	}
	return ui.Truncate(label+"  "+msg, width)
}

func (m *Model) updateLogsKey(msg tea.KeyMsg, action string) tea.Cmd {
	entries := m.filteredLogs()
	n := len(entries)
	h := max(1, m.contentHeight()-1)
	maxTop := max(0, n-h)

	if m.logFollow {
		m.logTop = maxTop
	}

	switch {
	case key.Matches(msg, m.keys.Confirm):
		var lines []string
		for _, entry := range entries {
			lines = append(lines, strings.ToLower(ui.LogLevelLabel(int32(entry.Level)))+"  "+entry.Message)
		}
		m.openDetail("Logs (snapshot)", strings.Join(lines, "\n"))
		if m.logFollow {
			m.detail.offset = max(0, len(m.detailLines())-max(1, m.contentHeight()-1))
		} else {
			m.detail.offset = len(strings.Split(wrapText(strings.Join(lines[:clamp(m.logTop, 0, len(lines))], "\n"), m.width), "\n")) - 1
		}
	case action == "gg" || key.Matches(msg, m.keys.Top):
		m.logFollow = false
		m.logTop = 0
	case action == "G" || key.Matches(msg, m.keys.Bottom):
		m.logFollow = true
		m.logTop = maxTop
	case m.isUp(msg):
		m.logFollow = false
		m.logTop = clamp(m.logTop-1, 0, maxTop)
	case m.isDown(msg):
		m.logTop = clamp(m.logTop+1, 0, maxTop)
		m.logFollow = m.logTop >= maxTop
	case key.Matches(msg, m.keys.PageUp):
		m.logFollow = false
		m.logTop = clamp(m.logTop-pageStep(h), 0, maxTop)
	case key.Matches(msg, m.keys.PageDown):
		m.logTop = clamp(m.logTop+pageStep(h), 0, maxTop)
		m.logFollow = m.logTop >= maxTop
	case key.Matches(msg, m.keys.HalfUp):
		m.logFollow = false
		m.logTop = clamp(m.logTop-halfPageStep(h), 0, maxTop)
	case key.Matches(msg, m.keys.HalfDown):
		m.logTop = clamp(m.logTop+halfPageStep(h), 0, maxTop)
		m.logFollow = m.logTop >= maxTop
	case action == "c":
		if m.session != nil {
			sess := m.session
			return func() tea.Msg {
				return actionResultMsg{kind: "clearLogs", err: sess.ClearLogs()}
			}
		}
	case key.Matches(msg, m.keys.Filter):
		cur := m.logThreshold()
		next := logLevelCycle[0]
		for i, lv := range logLevelCycle {
			if lv == cur {
				next = logLevelCycle[(i+1)%len(logLevelCycle)]
				break
			}
		}
		m.logUseDefault = false
		m.logLevelFilter = next
	}
	return nil
}
