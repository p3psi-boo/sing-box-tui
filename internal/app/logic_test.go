package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

func TestVisibleWindowFollowsCursor(t *testing.T) {
	start, end, off := visibleWindow(20, 0, 0, 5)
	if start != 0 || end != 5 || off != 0 {
		t.Fatalf("top: %d %d %d", start, end, off)
	}
	start, end, off = visibleWindow(20, 19, 0, 5)
	if start != 15 || end != 20 || off != 15 {
		t.Fatalf("bottom: %d %d %d", start, end, off)
	}
	start, end, off = visibleWindow(20, 7, 5, 5)
	if start != 5 || end != 10 || off != 5 {
		t.Fatalf("sticky: %d %d %d", start, end, off)
	}
	start, end, off = visibleWindow(3, 1, 0, 10)
	if start != 0 || end != 3 || off != 0 {
		t.Fatalf("short: %d %d %d", start, end, off)
	}
}

func sampleGroupsModel() Model {
	m := NewModel("", &config.Config{})
	m.width = 72
	m.snapshot.Connected = true
	m.snapshot.GroupsLoaded = true
	m.snapshot.Groups = []*daemon.Group{
		{
			Tag:        "proxy",
			Type:       "selector",
			Selectable: true,
			Selected:   "jp-1",
			Items: []*daemon.GroupItem{
				{Tag: "jp-1", Type: "vmess", UrlTestDelay: 80},
				{Tag: "us-1", Type: "trojan", UrlTestDelay: 1200},
			},
		},
		{
			Tag:      "auto",
			Type:     "urltest",
			Selected: "us-1",
			Items:    []*daemon.GroupItem{{Tag: "us-1", Type: "trojan", UrlTestDelay: 42}},
		},
		{
			Tag:      "backup",
			Type:     "fallback",
			Selected: "direct",
			Items:    []*daemon.GroupItem{{Tag: "direct", Type: "direct"}},
		},
	}
	return m
}

func TestViewGroupsLayout(t *testing.T) {
	m := sampleGroupsModel()
	got := ansi.Strip(m.viewGroups(20))
	if strings.Contains(got, "──") {
		t.Fatalf("dividers still present:\n%s", got)
	}
	if !strings.HasPrefix(got, "> ▸ proxy") {
		t.Fatalf("cursor/chevron header:\n%s", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("collapsed should be 3 rows, got %d:\n%s", len(lines), got)
	}
	selCol := strings.Index(lines[0], "Selector")
	urlCol := strings.Index(lines[1], "URLTest")
	fbCol := strings.Index(lines[2], "Fallback")
	if selCol < 0 || selCol != urlCol || selCol != fbCol {
		t.Fatalf("types not aligned (%d %d %d):\n%s", selCol, urlCol, fbCol, got)
	}
	if !strings.Contains(lines[0], "jp-1") || !strings.Contains(lines[0], "80ms") {
		t.Fatalf("selected/delay missing:\n%s", lines[0])
	}
	if w := ui.DisplayWidth(lines[0]); w != m.width {
		t.Fatalf("row width %d want %d: %q", w, m.width, lines[0])
	}
	for i, line := range lines {
		if ui.DisplayWidth(line) != m.width {
			t.Fatalf("row %d width %d: %q", i, ui.DisplayWidth(line), line)
		}
	}

	m.expandOverride["proxy"] = true
	got = ansi.Strip(m.viewGroups(20))
	lines = strings.Split(got, "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], "▾") {
		t.Fatalf("expanded:\n%s", got)
	}
	if !strings.HasPrefix(lines[1], "    jp-1") {
		t.Fatalf("item indent:\n%s", lines[1])
	}
	if !strings.Contains(lines[1], "VMess") || !strings.Contains(lines[1], "✓") {
		t.Fatalf("item meta:\n%s", lines[1])
	}
}

func TestCurrentGroupRowsApplySelectionAcrossDelay(t *testing.T) {
	m := sampleGroupsModel()
	cols := m.groupCols()
	group := m.snapshot.Groups[0]

	header := m.renderGroupHeader(group, false, cols)
	wantHeader := ui.SelectedStyle.Render("> " + strings.TrimPrefix(ansi.Strip(header), "  "))
	if got := m.renderGroupHeader(group, true, cols); got != wantHeader {
		t.Fatalf("selected header style does not cover the full row\ngot:  %q\nwant: %q", got, wantHeader)
	}

	item := m.renderGroupItem(group, group.Items[0], false, cols)
	wantItem := ui.SelectedStyle.Render("> " + strings.TrimPrefix(ansi.Strip(item), "  "))
	if got := m.renderGroupItem(group, group.Items[0], true, cols); got != wantItem {
		t.Fatalf("selected item style does not cover the full row\ngot:  %q\nwant: %q", got, wantItem)
	}
}

func TestGroupRowsDoNotAutoExpand(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.snapshot.Groups = []*daemon.Group{
		{
			Tag:        "proxy",
			Type:       "selector",
			Selectable: true,
			Selected:   "a",
			IsExpand:   false,
			Items: []*daemon.GroupItem{
				{Tag: "a", UrlTestDelay: 100},
				{Tag: "b", UrlTestDelay: 200},
			},
		},
	}
	m.groupsCursor = 0
	rows := m.buildGroupRows()
	if len(rows) != 1 || !rows[0].IsHeader {
		t.Fatalf("collapsed cursor should stay on header, got %+v", rows)
	}
	m.expandOverride["proxy"] = true
	rows = m.buildGroupRows()
	if len(rows) != 3 {
		t.Fatalf("expanded rows = %d", len(rows))
	}
}

func TestFilterLogsIsThreshold(t *testing.T) {
	entries := []client.LogEntry{
		{Level: daemon.LogLevel_ERROR, Message: "e"},
		{Level: daemon.LogLevel_WARN, Message: "w"},
		{Level: daemon.LogLevel_INFO, Message: "i"},
		{Level: daemon.LogLevel_DEBUG, Message: "d"},
	}
	got := filterLogs(entries, int32(daemon.LogLevel_WARN))
	if len(got) != 2 || got[0].Message != "e" || got[1].Message != "w" {
		t.Fatalf("warn+ = %+v", got)
	}
}

func sampleConnModel() Model {
	m := NewModel("", &config.Config{})
	m.width = 72
	m.snapshot.Connected = true
	m.snapshot.ConnectionsLoaded = true
	m.connFilter = connAll
	m.snapshot.Connections = map[string]*client.ConnectionRow{
		"1": {
			Connection: &daemon.Connection{
				Id:          "1",
				Domain:      "example.com",
				Destination: "1.2.3.4:443",
				Network:     "tcp",
				Outbound:    "jp-1",
				Source:      "10.0.0.2:1234",
			},
			DownlinkRate: 1_200_000,
			UplinkRate:   800,
		},
		"2": {
			Connection: &daemon.Connection{
				Id:          "2",
				Domain:      "idle.test",
				Destination: "9.9.9.9:443",
				Network:     "udp",
				Outbound:    "direct",
			},
		},
		"3": {
			Connection: &daemon.Connection{
				Id:          "3",
				Domain:      "old.example.com",
				Destination: "8.8.8.8:443",
				Network:     "tcp",
				Outbound:    "jp-1",
			},
			ClosedAt: 9,
		},
	}
	return m
}

func TestViewConnectionsLayout(t *testing.T) {
	m := sampleConnModel()
	got := ansi.Strip(m.viewConnections(20))
	if !strings.Contains(got, "example.com:443") {
		t.Fatalf("dest:\n%s", got)
	}
	if !strings.Contains(got, "↓1.2Mb") || !strings.Contains(got, "↑800b") {
		t.Fatalf("rates:\n%s", got)
	}
	if !strings.Contains(got, "closed") {
		t.Fatalf("closed state:\n%s", got)
	}
	lines := strings.Split(got, "\n")
	var list []string
	for _, line := range lines {
		if strings.Contains(line, "──") {
			break
		}
		if strings.Contains(line, "example.com") || strings.Contains(line, "idle.test") || strings.Contains(line, "old.example") {
			list = append(list, line)
		}
	}
	if len(list) < 3 {
		t.Fatalf("list rows:\n%s", got)
	}
	jp := strings.Index(list[0], "jp-1")
	if jp < 0 {
		t.Fatalf("outbound:\n%s", list[0])
	}
	tcp := strings.Index(list[0], "tcp")
	udp := -1
	for _, line := range list {
		if i := strings.Index(line, "udp"); i >= 0 {
			udp = i
		}
	}
	if tcp < 0 || udp < 0 || tcp != udp {
		t.Fatalf("network not aligned tcp=%d udp=%d:\n%s", tcp, udp, got)
	}

	m.width = 40
	got = ansi.Strip(m.viewConnections(20))
	if !strings.Contains(got, "closed") && !strings.Contains(got, "×") {
		t.Fatalf("narrow closed mark:\n%s", got)
	}
}

func TestViewLogsLayout(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.width = 40
	m.snapshot.Connected = true
	m.snapshot.Logs = []client.LogEntry{
		{Level: daemon.LogLevel_ERROR, Message: "boom"},
		{Level: daemon.LogLevel_INFO, Message: "started"},
		{Level: daemon.LogLevel_DEBUG, Message: "noise"},
	}
	got := ansi.Strip(m.viewLogs(10))
	if !strings.HasPrefix(strings.Split(got, "\n")[1], "error  boom") {
		t.Fatalf("error line: %q", got)
	}
	if !strings.Contains(got, "info   started") {
		t.Fatalf("info line: %q", got)
	}

	m.logFollow = false
	m.logTop = 0
	m.snapshot.Logs = make([]client.LogEntry, 0, 8)
	for i := 0; i < 8; i++ {
		m.snapshot.Logs = append(m.snapshot.Logs, client.LogEntry{Level: daemon.LogLevel_INFO, Message: fmt.Sprintf("l%d", i)})
	}
	got = ansi.Strip(m.viewLogs(4))
	if !strings.Contains(got, "newer") {
		t.Fatalf("newer cue:\n%s", got)
	}

	m.logLevelFilter = int32(daemon.LogLevel_ERROR)
	m.logUseDefault = false
	got = ansi.Strip(m.viewLogs(4))
	if !strings.Contains(got, "No entries at this level") {
		t.Fatalf("empty filter: %q", got)
	}
}

func TestConnDetailLines(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.width = 40
	lines := m.connDetailLines(&client.ConnectionRow{
		Connection: &daemon.Connection{
			Domain:      "example.com",
			Destination: "1.2.3.4:443",
			Network:     "tcp",
			Outbound:    "jp-1",
			Source:      "10.0.0.2:1234",
		},
	})
	if len(lines) < 3 {
		t.Fatalf("lines = %v", lines)
	}
	if !strings.Contains(lines[0], "example.com") {
		t.Fatalf("title %q", lines[0])
	}
}

func TestFilterConnectionsActiveDefault(t *testing.T) {
	conns := map[string]*client.ConnectionRow{
		"1": {Connection: &daemon.Connection{Id: "1", Domain: "a.com", CreatedAt: 2}, DownlinkRate: 10},
		"2": {Connection: &daemon.Connection{Id: "2", Domain: "b.com", CreatedAt: 1}, ClosedAt: 9},
	}
	got := filterConnections(conns, "", connActive)
	if len(got) != 1 || got[0].Connection.Id != "1" {
		t.Fatalf("active = %+v", got)
	}
	got = filterConnections(conns, "b.com", connAll)
	if len(got) != 1 || got[0].Connection.Id != "2" {
		t.Fatalf("search closed = %+v", got)
	}
}

func TestConnectionDestPrefersDomainPort(t *testing.T) {
	c := &daemon.Connection{Domain: "example.com", Destination: "1.2.3.4:443"}
	if got := connectionDest(c); got != "example.com:443" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectErrorHint(t *testing.T) {
	err := errors.New("clash api 400 Bad Request: Must be a Selector")
	if got := selectErrorHint(err, true); got != "Clash API can only switch Selector groups" {
		t.Fatalf("got %q", got)
	}
	if got := selectErrorHint(err, false); got != err.Error() {
		t.Fatalf("passthrough %q", got)
	}
}

func TestSlugID(t *testing.T) {
	used := map[string]bool{"home": true}
	if got := slugID("Home Router", used); got != "home-router" {
		t.Fatalf("got %q", got)
	}
	if got := slugID("Home", used); got != "home-2" {
		t.Fatalf("collision got %q", got)
	}
	if got := slugID("  ", nil); got != "server" {
		t.Fatalf("empty got %q", got)
	}
}
