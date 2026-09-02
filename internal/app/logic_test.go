package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
)

func TestVisibleGroupsAccountsForDividers(t *testing.T) {
	rows := []GroupRow{
		{GroupIndex: 0, ItemIndex: -1, IsHeader: true},
		{GroupIndex: 0, ItemIndex: 0},
		{GroupIndex: 1, ItemIndex: -1, IsHeader: true},
		{GroupIndex: 1, ItemIndex: 0},
	}
	start, end, _ := visibleGroups(rows, 0, 0, 3)
	if start != 0 || end != 2 {
		t.Fatalf("top window %d %d", start, end)
	}
	start, end, off := visibleGroups(rows, 2, 0, 3)
	if start != 1 || end != 3 || off != 1 {
		t.Fatalf("scrolled to header %d %d %d", start, end, off)
	}
}

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
