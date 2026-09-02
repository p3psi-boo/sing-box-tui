package app

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

func connectionDest(c *daemon.Connection) string {
	if c == nil {
		return ""
	}
	if c.Domain != "" {
		if port := destPort(c.Destination); port != "" && !strings.Contains(c.Domain, ":") {
			return c.Domain + ":" + port
		}
		return c.Domain
	}
	return c.Destination
}

func destPort(destination string) string {
	if destination == "" {
		return ""
	}
	if strings.HasPrefix(destination, "[") {
		if i := strings.LastIndex(destination, "]:"); i >= 0 {
			return destination[i+2:]
		}
		return ""
	}
	if i := strings.LastIndex(destination, ":"); i >= 0 && strings.Count(destination, ":") == 1 {
		return destination[i+1:]
	}
	return ""
}

func filterConnections(conns map[string]*client.ConnectionRow, query string, filter connStateFilter) []*client.ConnectionRow {
	query = strings.ToLower(strings.TrimSpace(query))
	rows := make([]*client.ConnectionRow, 0, len(conns))
	for _, row := range conns {
		if row == nil || row.Connection == nil {
			continue
		}
		closed := row.ClosedAt > 0
		switch filter {
		case connActive:
			if closed {
				continue
			}
		case connClosed:
			if !closed {
				continue
			}
		}
		if query != "" {
			c := row.Connection
			hay := strings.ToLower(c.Source + " " + c.Destination + " " + c.Domain + " " + c.Outbound + " " + c.Protocol + " " + c.Network)
			if !strings.Contains(hay, query) {
				continue
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		ri := rows[i].UplinkRate + rows[i].DownlinkRate
		rj := rows[j].UplinkRate + rows[j].DownlinkRate
		if ri != rj {
			return ri > rj
		}
		return rows[i].Connection.CreatedAt > rows[j].Connection.CreatedAt
	})
	return rows
}

func (m Model) filteredConnections() []*client.ConnectionRow {
	return filterConnections(m.snapshot.Connections, m.connSearch.Value(), m.connFilter)
}

type connLayout struct {
	headH int
	listH int
	pane  []string
}

func (m Model) connLayout(height int, rows []*client.ConnectionRow) connLayout {
	lay := connLayout{}
	if m.mode == modeSearch || m.connSearch.Value() != "" {
		lay.headH = 1
	}
	remain := height - lay.headH
	if remain < 1 {
		remain = 1
	}
	if len(rows) > 0 {
		idx := clamp(m.connCursor, 0, len(rows)-1)
		pane := m.connDetailLines(rows[idx])
		maxPane := min(len(pane), 10)
		if remain >= 6 {
			maxPane = min(maxPane, remain-4)
			if maxPane >= 2 {
				if len(pane) > maxPane {
					pane = pane[:maxPane]
				}
				lay.pane = pane
			}
		}
	}
	lay.listH = remain
	if len(lay.pane) > 0 {
		lay.listH = remain - len(lay.pane) - 1
	}
	if lay.listH < 1 {
		lay.listH = 1
	}
	return lay
}

func (m Model) viewConnections(height int) string {
	if !m.connected() {
		return m.viewDisconnected("")
	}
	if m.snapshot.ConnectionsBlocked {
		msg := m.snapshot.ConnectionsError
		if msg == "" {
			msg = "Clash API is not enabled on this instance"
		}
		return ui.WarningStyle.Render(msg)
	}
	if !m.snapshot.ConnectionsLoaded {
		return ui.DimStyle.Render("Waiting for connections")
	}

	rows := m.filteredConnections()
	lay := m.connLayout(height, rows)
	var top []string
	if m.mode == modeSearch {
		top = append(top, m.connSearch.View())
	} else if q := m.connSearch.Value(); q != "" {
		top = append(top, ui.DimStyle.Render("/"+q))
	}

	if len(rows) == 0 {
		if m.connSearch.Value() != "" {
			top = append(top, ui.DimStyle.Render("No matches"))
		} else {
			top = append(top, ui.DimStyle.Render("No connections"))
		}
		return strings.Join(top, "\n")
	}

	cols := connColsFor(rows, m.width)
	start, end, _ := visibleWindow(len(rows), m.connCursor, m.connOffset, lay.listH)
	for i := start; i < end; i++ {
		top = append(top, m.renderConnRow(rows[i], i == m.connCursor, cols))
	}
	list := lipgloss.NewStyle().Width(m.width).Height(lay.headH + lay.listH).MaxHeight(lay.headH + lay.listH).Render(strings.Join(top, "\n"))
	if len(lay.pane) == 0 {
		return list
	}
	return list + "\n" + ui.Divider(m.width) + "\n" + strings.Join(lay.pane, "\n")
}

type connCols struct {
	destW  int
	outW   int
	netW   int
	stateW int
}

func connColsFor(rows []*client.ConnectionRow, width int) connCols {
	var c connCols
	for _, row := range rows {
		if row == nil || row.Connection == nil {
			continue
		}
		if w := ui.DisplayWidth(connectionDest(row.Connection)); w > c.destW {
			c.destW = w
		}
		if w := ui.DisplayWidth(row.Connection.Outbound); w > c.outW {
			c.outW = w
		}
		if w := ui.DisplayWidth(row.Connection.Network); w > c.netW {
			c.netW = w
		}
		if row.ClosedAt > 0 && c.stateW < 6 {
			c.stateW = 6
		}
	}
	if c.outW > 16 {
		c.outW = 16
	}
	if c.netW > 4 {
		c.netW = 4
	}

	remain := func() int {
		fixed := 2 + 1 + 7 + 2 + 7
		if c.outW > 0 {
			fixed += 2
		}
		if c.netW > 0 {
			fixed += 2
		}
		if c.stateW > 0 {
			fixed += 2
		}
		r := width - fixed - c.outW - c.netW - c.stateW
		if r < 0 {
			return 0
		}
		return r
	}
	if remain() < 8 && c.stateW >= 6 {
		c.stateW = 1
	}
	if r := remain(); r < 8 && c.outW > 0 {
		c.outW -= min(c.outW, 8-r)
	}
	if r := remain(); r < 8 && c.netW > 0 {
		c.netW -= min(c.netW, 8-r)
	}
	r := remain()
	if c.destW > r {
		c.destW = r
	}
	if c.destW < 1 {
		c.destW = 1
	}
	return c
}

func (c connCols) ident(cursor, dest, out, net, state string) string {
	s := cursor + ui.PadRight(dest, c.destW)
	if c.outW > 0 {
		s += "  " + ui.PadRight(out, c.outW)
	}
	if c.netW > 0 {
		s += "  " + ui.PadRight(net, c.netW)
	}
	if c.stateW > 0 {
		s += "  " + ui.PadRight(state, c.stateW)
	}
	return s
}

func rateArrow(arrow string, bps int64) string {
	s := ui.FormatRateArrow(arrow, bps)
	if bps <= 0 {
		return ui.DimStyle.Render(s)
	}
	return s
}

func (m Model) renderConnRow(row *client.ConnectionRow, current bool, cols connCols) string {
	c := row.Connection
	net := c.Network
	if net != "" {
		net = ui.DimStyle.Render(net)
	}
	state := ""
	if row.ClosedAt > 0 {
		if cols.stateW >= 6 {
			state = ui.DimStyle.Render("closed")
		} else if cols.stateW > 0 {
			state = ui.DimStyle.Render("×")
		}
	}
	left := cols.ident(cursorPrefix(current), connectionDest(c), c.Outbound, net, state)
	right := rateArrow("↓", row.DownlinkRate) + "  " + rateArrow("↑", row.UplinkRate)
	line := ui.FitRow(left, right, m.width)
	if row.ClosedAt > 0 {
		line = ui.DimStyle.Render(line)
	}
	if current {
		return ui.SelectedStyle.Render(line)
	}
	return line
}

func (m *Model) updateConnectionsKey(msg tea.KeyMsg, action string) tea.Cmd {
	if key.Matches(msg, m.keys.Search) {
		m.mode = modeSearch
		return m.connSearch.Focus()
	}
	if key.Matches(msg, m.keys.Back) && m.connSearch.Value() != "" {
		m.connSearch.SetValue("")
		m.connSearch.Blur()
		m.clampWindows()
		return nil
	}

	rows := m.filteredConnections()
	n := len(rows)
	lay := m.connLayout(m.contentHeight(), rows)
	move := func(next int) {
		if n == 0 {
			return
		}
		m.connCursor = clamp(next, 0, n-1)
		lay := m.connLayout(m.contentHeight(), rows)
		_, _, m.connOffset = visibleWindow(n, m.connCursor, m.connOffset, lay.listH)
	}

	switch {
	case action == "gg" || key.Matches(msg, m.keys.Top):
		move(0)
	case action == "G" || key.Matches(msg, m.keys.Bottom):
		move(n - 1)
	case m.isDown(msg):
		move(m.connCursor + 1)
	case m.isUp(msg):
		move(m.connCursor - 1)
	case key.Matches(msg, m.keys.PageDown):
		move(m.connCursor + pageStep(lay.listH))
	case key.Matches(msg, m.keys.PageUp):
		move(m.connCursor - pageStep(lay.listH))
	case key.Matches(msg, m.keys.HalfDown):
		move(m.connCursor + halfPageStep(lay.listH))
	case key.Matches(msg, m.keys.HalfUp):
		move(m.connCursor - halfPageStep(lay.listH))
	case key.Matches(msg, m.keys.Filter):
		m.connFilter = (m.connFilter + 1) % 3
		m.connCursor = 0
		m.clampWindows()
	case key.Matches(msg, m.keys.CloseOne):
		if m.session != nil && n > 0 && m.connCursor < n {
			id := rows[m.connCursor].Connection.Id
			sess := m.session
			return func() tea.Msg {
				return actionResultMsg{kind: "close", err: sess.CloseConnection(id)}
			}
		}
	case key.Matches(msg, m.keys.CloseAll):
		m.confirm = confirmCloseAll
		m.returnMode = modeNormal
		m.mode = modeConfirm
	}
	return nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.mode = modeNormal
		m.connSearch.Blur()
		m.connSearch.SetValue("")
		m.clampWindows()
		return m, nil
	case key.Matches(msg, m.keys.Confirm):
		m.mode = modeNormal
		m.connSearch.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.connSearch, cmd = m.connSearch.Update(msg)
	m.connCursor = 0
	m.clampWindows()
	return m, cmd
}

func (m Model) connDetailLines(row *client.ConnectionRow) []string {
	if row == nil || row.Connection == nil {
		return nil
	}
	c := row.Connection
	state := "active"
	if row.ClosedAt > 0 {
		state = "closed"
	}
	var metaBits []string
	if c.Network != "" {
		metaBits = append(metaBits, c.Network)
	}
	if c.Outbound != "" {
		metaBits = append(metaBits, c.Outbound)
	}
	metaBits = append(metaBits, state)
	lines := []string{connectionDest(c), ui.DimStyle.Render(strings.Join(metaBits, "  "))}

	type kv struct{ k, v string }
	var pairs []kv
	add := func(k, v string) {
		if v != "" {
			pairs = append(pairs, kv{k, v})
		}
	}
	add("source", c.Source)
	add("destination", c.Destination)
	add("protocol", c.Protocol)
	add("rule", c.Rule)
	if len(c.ChainList) > 0 {
		chain := append([]string(nil), c.ChainList...)
		for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
			chain[i], chain[j] = chain[j], chain[i]
		}
		add("chain", strings.Join(chain, " / "))
	}
	add("inbound", strings.TrimSpace(c.InboundType+" "+c.Inbound))
	if c.ProcessInfo != nil {
		proc := path.Base(c.ProcessInfo.ProcessPath)
		if proc == "" || proc == "." {
			proc = c.ProcessInfo.ProcessPath
		}
		if c.ProcessInfo.ProcessId > 0 {
			proc = fmt.Sprintf("%s (%d)", proc, c.ProcessInfo.ProcessId)
		}
		add("process", proc)
	}
	add("traffic", "↑ "+ui.FormatBytes(uint64(c.UplinkTotal))+"  ↓ "+ui.FormatBytes(uint64(c.DownlinkTotal)))
	if c.CreatedAt > 0 {
		add("created", time.UnixMilli(c.CreatedAt).Format("15:04:05"))
	}
	keyW := 0
	for _, p := range pairs {
		if w := ui.DisplayWidth(p.k); w > keyW {
			keyW = w
		}
	}
	for _, p := range pairs {
		lines = append(lines, ui.Truncate(ui.PadRight(ui.DimStyle.Render(p.k), keyW)+"  "+p.v, m.width))
	}
	return lines
}
