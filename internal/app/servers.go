package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var errNoServer = errors.New("no server configured")

type serverForm struct {
	typ    string
	focus  int
	inputs []textinput.Model
}

func newInput(placeholder string, password bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	ti.CharLimit = 256
	if password {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	return ti
}

func newServerForm() serverForm {
	f := serverForm{typ: "direct", focus: 1}
	f.rebuild(nil)
	return f
}

func (f *serverForm) rebuild(prev map[string]string) {
	if prev == nil {
		prev = map[string]string{}
		for i, label := range f.labels() {
			if i == 0 {
				continue
			}
			idx := i - 1
			if idx >= 0 && idx < len(f.inputs) {
				prev[label] = f.inputs[idx].Value()
			}
		}
	}
	labels := formLabels(f.typ)
	f.inputs = make([]textinput.Model, 0, len(labels)-1)
	for i, label := range labels {
		if i == 0 {
			continue
		}
		password := label == "secret"
		ph := placeholderFor(f.typ, label)
		in := newInput(ph, password)
		if v, ok := prev[label]; ok {
			in.SetValue(v)
		} else if v, ok := prev[aliasLabel(label)]; ok {
			in.SetValue(v)
		}
		f.inputs = append(f.inputs, in)
	}
	if f.focus >= len(labels) {
		f.focus = 0
	}
	f.applyFocus()
}

func aliasLabel(label string) string {
	if label == "host" {
		return "address"
	}
	if label == "address" {
		return "host"
	}
	return label
}

func formLabels(typ string) []string {
	if typ == "ssh" {
		return []string{"type", "name", "host", "user", "port", "identity", "remote", "secret"}
	}
	return []string{"type", "name", "address", "secret"}
}

func placeholderFor(typ, label string) string {
	switch label {
	case "name":
		return ""
	case "address":
		return "127.0.0.1:9090"
	case "host":
		return "vps.example.com"
	case "user":
		return "root"
	case "port":
		return "22"
	case "identity":
		return "~/.ssh/id_ed25519"
	case "remote":
		return "127.0.0.1:9090"
	case "secret":
		return "optional"
	}
	return ""
}

func (f serverForm) labels() []string {
	return formLabels(f.typ)
}

func (f *serverForm) applyFocus() {
	for i := range f.inputs {
		if i+1 == f.focus {
			f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
}

func (f *serverForm) fieldCount() int {
	return len(f.labels())
}

func (m *Model) openForm() {
	m.form = newServerForm()
	m.mode = modeServerForm
}

func (m Model) viewServerPicker(height int) string {
	if len(m.cfg.Servers) == 0 {
		return m.viewSetup()
	}
	n := len(m.cfg.Servers)
	start, end, _ := visibleWindow(n, m.settingsCursor, m.settingsOffset, height)
	var lines []string
	for i := start; i < end; i++ {
		s := m.cfg.Servers[i]
		prefix := "  "
		if i == m.settingsCursor {
			prefix = "> "
		}
		active := ""
		if s.ID == m.cfg.Active {
			active = " *"
		}
		left := prefix + s.DisplayName()
		right := s.Type
		if right == "" {
			right = "direct"
		}
		if s.Type == "ssh" {
			right += "  " + s.SSH.Host
		} else if s.Address != "" {
			right += "  " + s.Address
		}
		line := ui.FitRow(left+active, right, m.width)
		if i == m.settingsCursor {
			line = ui.SelectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewServerForm(height int) string {
	labels := m.form.labels()
	labelWidth := 10
	var lines []string
	lines = append(lines, "Add server", "")
	for i, label := range labels {
		prefix := "  "
		if i == m.form.focus {
			prefix = "> "
		}
		head := fmt.Sprintf("%s%-*s", prefix, labelWidth, label)
		var value string
		if i == 0 {
			value = m.form.typ
		} else {
			idx := i - 1
			if idx >= 0 && idx < len(m.form.inputs) {
				value = m.form.inputs[idx].View()
			}
		}
		lines = append(lines, head+" "+value)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) updateServers(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Servers):
		m.mode = modeNormal
	case key.Matches(msg, m.keys.Help):
		m.returnMode = modeServers
		m.mode = modeHelp
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Add):
		m.openForm()
	case key.Matches(msg, m.keys.Delete):
		if len(m.cfg.Servers) > 0 {
			m.deleteIndex = m.settingsCursor
			m.confirm = confirmDeleteServer
			m.returnMode = modeServers
			m.mode = modeConfirm
		}
	case key.Matches(msg, m.keys.Top) || m.resolveKey(msg) == "gg":
		m.settingsCursor = 0
	case key.Matches(msg, m.keys.Bottom) || msg.String() == "G":
		if len(m.cfg.Servers) > 0 {
			m.settingsCursor = len(m.cfg.Servers) - 1
		}
	case m.isDown(msg):
		if m.settingsCursor < len(m.cfg.Servers)-1 {
			m.settingsCursor++
		}
	case m.isUp(msg):
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case key.Matches(msg, m.keys.Confirm):
		return m, m.activateServer(m.settingsCursor)
	}
	m.clampWindows()
	return m, nil
}

func (m *Model) activateServer(index int) tea.Cmd {
	if index < 0 || index >= len(m.cfg.Servers) {
		return nil
	}
	s := m.cfg.Servers[index]
	m.cfg.Active = s.ID
	_ = config.Save(m.cfgPath, m.cfg)
	m.mode = modeNormal
	m.setStatus("switching to " + s.DisplayName())
	m.connectErr = ""
	return m.connectCmd()
}

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if len(m.cfg.Servers) == 0 {
			m.mode = modeNormal
		} else {
			m.mode = modeServers
		}
		return m, nil
	case "ctrl+t":
		m.toggleFormType()
		return m, nil
	case "tab", "down":
		m.form.focus = (m.form.focus + 1) % m.form.fieldCount()
		m.form.applyFocus()
		return m, nil
	case "shift+tab", "up":
		n := m.form.fieldCount()
		m.form.focus = (m.form.focus - 1 + n) % n
		m.form.applyFocus()
		return m, nil
	case "enter":
		cmd, err := m.submitForm()
		if err != nil {
			m.setStatus(err.Error())
			return m, nil
		}
		return m, cmd
	}

	if m.form.focus == 0 {
		if msg.String() == "t" {
			m.toggleFormType()
			return m, nil
		}
		return m, nil
	}
	idx := m.form.focus - 1
	if idx >= 0 && idx < len(m.form.inputs) {
		var cmd tea.Cmd
		m.form.inputs[idx], cmd = m.form.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) toggleFormType() {
	if m.form.typ == "ssh" {
		m.form.typ = "direct"
	} else {
		m.form.typ = "ssh"
	}
	m.form.rebuild(nil)
	m.form.focus = 0
}

func (m *Model) submitForm() (tea.Cmd, error) {
	values := map[string]string{}
	labels := m.form.labels()
	for i, label := range labels {
		if i == 0 {
			continue
		}
		idx := i - 1
		if idx >= 0 && idx < len(m.form.inputs) {
			values[label] = strings.TrimSpace(m.form.inputs[idx].Value())
		}
	}
	name := values["name"]
	s := config.Server{
		Type:   m.form.typ,
		Name:   name,
		Secret: values["secret"],
	}
	switch m.form.typ {
	case "ssh":
		s.SSH.Host = values["host"]
		s.SSH.User = values["user"]
		s.SSH.IdentityFile = values["identity"]
		s.SSH.Port = 22
		if values["port"] != "" {
			p, err := strconv.Atoi(values["port"])
			if err != nil || p <= 0 {
				return nil, fmt.Errorf("invalid port")
			}
			s.SSH.Port = p
		}
		s.RemoteAddress = values["remote"]
		if s.RemoteAddress == "" {
			s.RemoteAddress = "127.0.0.1:9090"
		}
		if name == "" {
			s.Name = s.SSH.Host
		}
	default:
		s.Type = "direct"
		s.Address = values["address"]
		if name == "" {
			s.Name = s.Address
		}
	}
	used := map[string]bool{}
	for _, existing := range m.cfg.Servers {
		used[existing.ID] = true
	}
	s.ID = slugID(s.Name, used)
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Type == "ssh" && s.SSH.IdentityFile == "" {
		return nil, fmt.Errorf("identity file is required")
	}
	m.cfg.Servers = append(m.cfg.Servers, s)
	if m.cfg.Active == "" {
		m.cfg.Active = s.ID
	}
	if err := config.Save(m.cfgPath, m.cfg); err != nil {
		m.cfg.Servers = m.cfg.Servers[:len(m.cfg.Servers)-1]
		return nil, err
	}
	m.settingsCursor = len(m.cfg.Servers) - 1
	m.mode = modeServers
	m.setStatus("added " + s.DisplayName())
	if m.cfg.Active == s.ID && !m.connected() {
		m.mode = modeNormal
		return m.connectCmd(), nil
	}
	return nil, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		cmd := m.performConfirm()
		return m, cmd
	case "n", "esc", "q":
		m.mode = m.returnMode
		m.confirm = confirmNone
		return m, nil
	}
	return m, nil
}

func (m *Model) performConfirm() tea.Cmd {
	kind := m.confirm
	m.confirm = confirmNone
	m.mode = m.returnMode
	switch kind {
	case confirmCloseAll:
		m.mode = modeNormal
		if m.session == nil {
			return nil
		}
		m.setStatus("closing all connections")
		sess := m.session
		return func() tea.Msg {
			return actionResultMsg{kind: "closeAll", err: sess.CloseAllConnections()}
		}
	case confirmDeleteServer:
		cmd := m.deleteServer(m.deleteIndex)
		if len(m.cfg.Servers) == 0 {
			m.mode = modeNormal
		} else {
			m.mode = modeServers
		}
		return cmd
	}
	return nil
}

func (m *Model) deleteServer(index int) tea.Cmd {
	if index < 0 || index >= len(m.cfg.Servers) {
		return nil
	}
	removed := m.cfg.Servers[index]
	wasActive := removed.ID == m.cfg.Active
	m.cfg.Servers = append(m.cfg.Servers[:index], m.cfg.Servers[index+1:]...)
	if wasActive {
		if len(m.cfg.Servers) > 0 {
			m.cfg.Active = m.cfg.Servers[0].ID
		} else {
			m.cfg.Active = ""
		}
	}
	if m.settingsCursor >= len(m.cfg.Servers) && m.settingsCursor > 0 {
		m.settingsCursor--
	}
	_ = config.Save(m.cfgPath, m.cfg)
	m.setStatus("removed " + removed.DisplayName())
	if wasActive && m.cfg.Active != "" {
		return m.connectCmd()
	}
	return nil
}

func slugID(name string, used map[string]bool) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	hyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if !hyphen && b.Len() > 0 {
			b.WriteRune('-')
			hyphen = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "server"
	}
	base := id
	for n := 2; used[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}
