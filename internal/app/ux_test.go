package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

func press(m Model, k string) Model {
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "enter":
		key = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		key = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		key = tea.KeyMsg{Type: tea.KeyTab}
	case "F2":
		key = tea.KeyMsg{Type: tea.KeyF2}
	}
	next, _ := m.Update(key)
	return next.(Model)
}

func TestConnectionSelectionSurvivesRateReorder(t *testing.T) {
	m := sampleConnModel()
	m.height = 24
	m.connCursor = 0
	wanted := m.filteredConnections()[0].Connection.Id
	snapshot := m.snapshot
	snapshot.Connections = map[string]*client.ConnectionRow{}
	for id, row := range m.snapshot.Connections {
		cp := *row
		snapshot.Connections[id] = &cp
	}
	snapshot.Connections["2"].DownlinkRate = 9_000_000
	next, _ := m.Update(sessionUpdateMsg(client.Update{Snapshot: snapshot}))
	m = next.(Model)
	if m.filteredConnections()[m.connCursor].Connection.Id != wanted {
		t.Fatal("selection moved to another connection")
	}
	m.updateConnectionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}, "p")
	snapshot.Connections = map[string]*client.ConnectionRow{}
	next, _ = m.Update(sessionUpdateMsg(client.Update{Snapshot: snapshot}))
	m = next.(Model)
	if len(m.filteredConnections()) != 3 || !m.connPaused {
		t.Fatal("paused list changed")
	}
}

func TestCloseOneCapturesTargetAndRequiresExplicitConfirmation(t *testing.T) {
	m := sampleConnModel()
	m.height = 24
	m.session = &client.Session{}
	m.updateConnectionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}, "x")
	if m.mode != modeConfirm || m.closeID != "1" {
		t.Fatalf("confirmation target: %s", m.closeID)
	}
	m = press(m, "enter")
	if m.mode != modeConfirm {
		t.Fatal("Enter confirmed destructive operation")
	}
	m = press(m, "esc")
	if m.mode != modeNormal {
		t.Fatal("cannot cancel")
	}
}

func TestNarrowFormErrorIsVisibleAndPersistent(t *testing.T) {
	m := NewModel(filepath.Join(t.TempDir(), "config.yaml"), &config.Config{})
	m.width = 40
	m.height = 10
	m.openForm()
	m = press(m, "enter")
	view := ansi.Strip(m.View())
	if m.form.labels()[m.form.focus] != "address" || !strings.Contains(view, "! Enter host:port") || !strings.Contains(view, "F2 details") {
		t.Fatalf("hidden validation:\n%s", view)
	}
	m.statusAt = time.Now().Add(-time.Hour)
	next, _ := m.Update(tickMsg(time.Now()))
	m = next.(Model)
	if m.statusMsg == "" {
		t.Fatal("error expired")
	}
	m = press(m, "F2")
	if !strings.Contains(m.detail.text, "127.0.0.1:9090") {
		t.Fatal("full error unavailable")
	}
	m = press(m, "esc")
	if m.mode != modeServerForm {
		t.Fatal("did not return to form")
	}
}

func TestSSHFormKeepsFocusedFieldOnscreen(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.width = 40
	m.height = 10
	m.openForm()
	m.toggleFormType()
	for i := range m.form.labels() {
		m.form.focus = i
		m.form.applyFocus()
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "> "+m.form.labels()[i]) {
			t.Fatalf("field %s hidden:\n%s", m.form.labels()[i], view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ui.DisplayWidth(line) > m.width {
				t.Fatalf("overflow: %q", line)
			}
		}
		if len(strings.Split(view, "\n")) > m.height {
			t.Fatalf("too tall:\n%s", view)
		}
	}
}

func TestEditingServerPreservesIdentityAndAdvancedSettings(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Active: "home", Servers: []config.Server{{ID: "home", Name: "Home", Type: "direct", API: "clash", TLS: true, Address: "localhost:9090", Secret: "secret", SSH: config.SSHConfig{KnownHostsFile: "custom"}}}}
	m := NewModel(filepath.Join(dir, "config.yaml"), cfg)
	m.width = 60
	m.editServer(0)
	values := m.form.values()
	values["address"] = "localhost:9091"
	m.form.rebuild(values)
	_, err := m.submitForm()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(m.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s := saved.Servers[0]
	if len(saved.Servers) != 1 || s.ID != "home" || s.Address != "localhost:9091" || s.API != "clash" || !s.TLS || s.Secret != "secret" || s.SSH.KnownHostsFile != "custom" {
		t.Fatalf("lost settings: %+v", s)
	}
}

func TestFailedSaveDoesNotMutateConfig(t *testing.T) {
	dir := t.TempDir()
	block := filepath.Join(dir, "file")
	if err := os.WriteFile(block, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	m := NewModel(filepath.Join(block, "config.yaml"), cfg)
	m.openForm()
	m.form.rebuild(map[string]string{"address": "127.0.0.1:9090"})
	if _, err := m.submitForm(); err == nil {
		t.Fatal("expected save failure")
	}
	if len(cfg.Servers) != 0 || cfg.Active != "" {
		t.Fatalf("config changed: %+v", cfg)
	}
	if m.form.values()["address"] == "" {
		t.Fatal("input lost")
	}
}

func TestFullDetailsPreserveLongFieldsAndTail(t *testing.T) {
	m := sampleConnModel()
	m.width = 24
	m.height = 10
	row := m.snapshot.Connections["1"]
	row.Connection.Rule = strings.Repeat("long-rule-", 20) + "TAIL"
	row.Connection.ChainList = []string{"one", "two", "three"}
	row.Connection.CreatedAt = 1000
	m.openDetail("Connection", strings.Join(m.connDetailLines(row), "\n"))
	if !strings.Contains(m.detail.text, "TAIL") || !strings.Contains(m.detail.text, "created") {
		t.Fatal("detail data truncated")
	}
	m.detail.offset = 1000
	view := ansi.Strip(m.viewDetail(7))
	if !strings.Contains(view, "created") {
		t.Fatalf("cannot reach bottom:\n%s", view)
	}
	m.openDetail("Logs", strings.Repeat("中", 100)+"END")
	if strings.Join(m.detailLines(), "") != m.detail.text {
		t.Fatal("wrapped text lost content")
	}
}

func TestDiscoveryPrefillDoesNotSaveOrInterruptEditing(t *testing.T) {
	m := NewModel(filepath.Join(t.TempDir(), "config.yaml"), &config.Config{})
	m.width = 60
	m.height = 24
	m.openForm()
	m.form.rebuild(map[string]string{"name": "My input"})
	next, _ := m.Update(discoveryResultMsg{results: []client.DiscoveredServer{{Address: "[::1]:9090", API: "grpc", NeedsSecret: true}}, err: errors.New("partial")})
	m = next.(Model)
	if m.mode != modeServerForm || m.form.values()["name"] != "My input" {
		t.Fatal("discovery interrupted typing")
	}
	m.mode = modeNormal
	m.useDiscovered()
	if m.form.values()["address"] != "[::1]:9090" || m.form.values()["api"] != "grpc" || m.form.labels()[m.form.focus] != "secret" {
		t.Fatal("bad discovery prefill")
	}
	if len(m.cfg.Servers) != 0 {
		t.Fatal("discovery saved without review")
	}
}

func TestHelpCanReachLastLine(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.width = 40
	m.height = 10
	m.mode = modeHelp
	m = press(m, "G")
	if !strings.Contains(ansi.Strip(m.viewHelp(7)), "close") {
		t.Fatal("help bottom unreachable")
	}
}

func TestLogSnapshotDoesNotChangeWithStream(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.width = 40
	m.height = 10
	m.snapshot.Logs = []client.LogEntry{{Level: daemon.LogLevel_ERROR, Message: strings.Repeat("full ", 100) + "TAIL"}}
	m.updateLogsKey(tea.KeyMsg{Type: tea.KeyEnter}, "enter")
	m.snapshot.Logs = nil
	if m.mode != modeDetail || !strings.HasSuffix(m.detail.text, "TAIL") {
		t.Fatal("log snapshot lost text")
	}
}

func TestSwitchingFormTypeRetainsDraftFields(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.openForm()
	m.form.rebuild(map[string]string{"name": "My host", "address": "127.0.0.1:9090", "tls": "true", "secret": "keep!"})
	m.toggleFormType()
	values := m.form.values()
	values["identity"] = "/keys/example"
	values["user"] = "admin"
	m.form.rebuild(values)
	m.toggleFormType()
	if m.form.values()["address"] != "127.0.0.1:9090" || m.form.values()["tls"] != "true" || m.form.values()["secret"] != "keep!" {
		t.Fatal("direct draft lost")
	}
	m.toggleFormType()
	if m.form.values()["identity"] != "/keys/example" || m.form.values()["user"] != "admin" {
		t.Fatal("SSH draft lost")
	}
}

func TestErrorDoesNotStealExclamationFromSecretInput(t *testing.T) {
	m := NewModel("", &config.Config{})
	m.openForm()
	m.setError("old error")
	for i, label := range m.form.labels() {
		if label == "secret" {
			m.form.focus = i
		}
	}
	m.form.applyFocus()
	m = press(m, "!")
	if m.mode != modeServerForm || m.form.values()["secret"] != "!" {
		t.Fatal("error shortcut stole password input")
	}
}
