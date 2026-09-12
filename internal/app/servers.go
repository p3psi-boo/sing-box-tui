package app

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

var errNoServer = errors.New("no server configured")

type serverForm struct {
	typ                   string
	focus                 int
	inputs                []textinput.Model
	original              config.Server
	editID                string
	draft                 map[string]string
	errorField, errorText string
}

func newInput(placeholder string, password bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = ""
	ti.CharLimit = 0
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
		return []string{"type", "name", "host", "user", "port", "identity", "remote", "secret", "api", "known_hosts"}
	}
	return []string{"type", "name", "address", "secret", "api", "tls"}
}

func placeholderFor(typ, label string) string {
	switch label {
	case "api":
		return "auto (auto / clash / grpc)"
	case "tls":
		return "false (true / false)"
	case "known_hosts":
		return "~/.ssh/known_hosts"
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
	m.resizeForm()
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
	title := "Add server"
	if m.form.editID != "" {
		title = "Edit server"
	}
	h := max(1, height-1)
	// A field and its validation message scroll together; focus is always visible.
	var rows []string
	focusRow := 0
	for i, label := range labels {
		if i == m.form.focus {
			focusRow = len(rows)
		}
		value := m.form.typ
		if i > 0 {
			value = m.form.inputs[i-1].View()
		}
		line := fmt.Sprintf("%s%-10s %s", cursorPrefix(i == m.form.focus), label, value)
		rows = append(rows, ui.Truncate(line, m.width))
		if label == m.form.errorField && m.form.errorText != "" {
			rows = append(rows, ui.ErrorStyle.Render(ui.Truncate("  ! "+m.form.errorText, m.width)))
		}
	}
	top := max(0, focusRow-h+1)
	if m.form.errorField == labels[m.form.focus] && h > 1 {
		top = max(0, focusRow-h+2)
	}
	end := min(len(rows), top+h)
	title += fmt.Sprintf(" · field %d/%d", m.form.focus+1, len(labels))
	return strings.Join(append([]string{ui.Truncate(title, m.width)}, rows[top:end]...), "\n")
}

func (m *Model) resizeForm() {
	for i := range m.form.inputs {
		m.form.inputs[i].Width = max(1, m.width-14)
	}
}

func (m *Model) editServer(index int) {
	if index < 0 || index >= len(m.cfg.Servers) {
		return
	}
	s := m.cfg.Servers[index]
	m.form = newServerForm()
	m.form.original = s
	m.form.editID = s.ID
	m.form.typ = s.Type
	if m.form.typ == "" {
		m.form.typ = "direct"
	}
	m.form.rebuild(map[string]string{"name": s.Name, "address": s.Address, "secret": s.Secret, "host": s.SSH.Host, "user": s.SSH.User, "port": strconv.Itoa(s.SSH.Port), "identity": s.SSH.IdentityFile, "remote": s.RemoteAddress, "api": s.API, "tls": strconv.FormatBool(s.TLS), "known_hosts": s.SSH.KnownHostsFile})
	if s.SSH.Port == 0 {
		for i, label := range m.form.labels()[1:] {
			if label == "port" {
				m.form.inputs[i].SetValue("")
			}
		}
	}
	m.mode = modeServerForm
	m.resizeForm()
}

func (f serverForm) values() map[string]string {
	values := map[string]string{}
	for i, label := range f.labels()[1:] {
		values[label] = f.inputs[i].Value()
	}
	return values
}

func (m *Model) formError(field, message string) error {
	m.setError(message)
	m.form.errorField = field
	m.form.errorText = message
	for i, label := range m.form.labels() {
		if label == field {
			m.form.focus = i
			break
		}
	}
	m.form.applyFocus()
	return errors.New(message)
}

func (m Model) updateServers(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Servers):
		m.mode = modeNormal
	case key.Matches(msg, m.keys.Help):
		m.returnMode = modeServers
		m.helpOffset = 0
		m.mode = modeHelp
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Add):
		m.openForm()
	case msg.String() == "e":
		m.editServer(m.settingsCursor)
	case key.Matches(msg, m.keys.Delete):
		if len(m.cfg.Servers) > 0 {
			m.deleteIndex = m.settingsCursor
			m.confirmOffset = 0
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
	next := *m.cfg
	next.Active = s.ID
	if err := config.Save(m.cfgPath, &next); err != nil {
		m.setError("Save failed: " + err.Error())
		return nil
	}
	*m.cfg = next
	m.connPaused = false
	m.pausedConnections = nil
	m.connID = ""
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
			if m.form.errorField == "" {
				m.setError("Save failed: " + err.Error())
			}
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
		m.form.errorField = ""
		m.form.errorText = ""
		var cmd tea.Cmd
		m.form.inputs[idx], cmd = m.form.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) toggleFormType() {
	if m.form.draft == nil {
		m.form.draft = map[string]string{}
	}
	for k, v := range m.form.values() {
		m.form.draft[k] = v
	}
	if m.form.typ == "ssh" {
		m.form.typ = "direct"
	} else {
		m.form.typ = "ssh"
	}
	m.form.rebuild(m.form.draft)
	m.form.errorField = ""
	m.form.errorText = ""
	m.form.focus = 0
	m.form.applyFocus()
	m.resizeForm()
}

func (m *Model) submitForm() (tea.Cmd, error) {
	m.form.errorField = ""
	m.form.errorText = ""
	values := m.form.values()
	for key, value := range values {
		if key != "secret" {
			values[key] = strings.TrimSpace(value)
		}
	}
	name := values["name"]
	s := m.form.original
	s.Type = m.form.typ
	s.Name = name
	s.Secret = values["secret"]
	s.API = values["api"]
	if s.API != "" && s.API != "auto" && s.API != "clash" && s.API != "grpc" {
		return nil, m.formError("api", "Use auto, clash or grpc")
	}
	if m.form.typ == "direct" {
		if values["address"] == "" {
			return nil, m.formError("address", "Enter host:port, e.g. 127.0.0.1:9090")
		}
		_, port, err := net.SplitHostPort(values["address"])
		n, e := strconv.Atoi(port)
		if err != nil || e != nil || n < 1 || n > 65535 {
			return nil, m.formError("address", "Use host:port with port 1–65535")
		}
		tls := values["tls"]
		if tls != "" && tls != "true" && tls != "false" {
			return nil, m.formError("tls", "Use true or false")
		}
		s.TLS = tls == "true"
		if s.TLS && s.API == "grpc" {
			return nil, m.formError("tls", "TLS is supported only with Clash API")
		}
	} else {
		for _, field := range []string{"host", "user", "identity"} {
			if values[field] == "" {
				return nil, m.formError(field, "Enter "+field)
			}
		}
		s.SSH.KnownHostsFile = values["known_hosts"]
	}
	switch m.form.typ {
	case "ssh":
		s.SSH.Host = values["host"]
		s.SSH.User = values["user"]
		s.SSH.IdentityFile = values["identity"]
		s.SSH.Port = 22
		if values["port"] != "" {
			p, err := strconv.Atoi(values["port"])
			if err != nil || p <= 0 || p > 65535 {
				return nil, m.formError("port", "Use a port from 1 to 65535")
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
	next := *m.cfg
	next.Servers = append([]config.Server(nil), m.cfg.Servers...)
	index := len(next.Servers)
	if m.form.editID != "" {
		index = -1
		for i, old := range next.Servers {
			if old.ID == m.form.editID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, errors.New("server no longer exists; reopen the server list")
		}
		s.ID = m.form.editID
	} else {
		used := map[string]bool{}
		for _, old := range next.Servers {
			used[old.ID] = true
		}
		s.ID = slugID(s.Name, used)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if index == len(next.Servers) {
		next.Servers = append(next.Servers, s)
	} else {
		next.Servers[index] = s
	}
	if next.Active == "" {
		next.Active = s.ID
	}
	if err := config.Save(m.cfgPath, &next); err != nil {
		return nil, err
	}
	*m.cfg = next
	m.statusError = false
	m.statusMsg = ""
	m.settingsCursor = index
	m.mode = modeServers
	m.setStatus("saved " + s.DisplayName())
	if m.cfg.Active == s.ID {
		m.mode = modeNormal
		m.connPaused = false
		m.pausedConnections = nil
		return m.connectCmd(), nil
	}
	return nil, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		cmd := m.performConfirm()
		return m, cmd
	case "n", "esc", "q":
		m.mode = m.returnMode
		m.confirm = confirmNone
		return m, nil
	default:
		m.confirmOffset = scrollKey(msg.String(), m.confirmOffset, len(m.confirmLines()), max(1, m.contentHeight()-1))
	}
	return m, nil
}

func (m *Model) performConfirm() tea.Cmd {
	kind := m.confirm
	m.confirm = confirmNone
	m.mode = m.returnMode
	switch kind {
	case confirmCloseOne:
		sess, id := m.closeSession, m.closeID
		if sess == nil || sess != m.session {
			m.setError("Server changed; select the connection again")
			return nil
		}
		return func() tea.Msg { return actionResultMsg{kind: "close", tag: id, err: sess.CloseConnection(id)} }
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
	next := *m.cfg
	next.Servers = append([]config.Server(nil), m.cfg.Servers...)
	next.Servers = append(next.Servers[:index], next.Servers[index+1:]...)
	wasActive := removed.ID == next.Active
	if wasActive {
		next.Active = ""
		if len(next.Servers) > 0 {
			next.Active = next.Servers[0].ID
		}
	}
	if err := config.Save(m.cfgPath, &next); err != nil {
		m.setError("Save failed: " + err.Error())
		return nil
	}
	*m.cfg = next
	m.settingsCursor = clamp(m.settingsCursor, 0, len(next.Servers)-1)
	m.setStatus("removed " + removed.DisplayName())
	if wasActive {
		m.connPaused = false
		m.pausedConnections = nil
		if next.Active != "" {
			return m.connectCmd()
		}
		m.snapshot = client.Snapshot{}
		m.session = nil
		mgr := m.manager
		return func() tea.Msg { return actionResultMsg{kind: "disconnect", err: mgr.Close()} }
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
