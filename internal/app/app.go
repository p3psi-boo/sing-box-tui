package app

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

type Page int

const (
	PageGroups Page = iota
	PageConnections
	PageLogs
)

const statusTTL = 3 * time.Second

type sessionUpdateMsg client.Update
type connectResultMsg struct{ err error }
type tickMsg time.Time
type actionResultMsg struct {
	kind string
	tag  string
	err  error
}

type confirmKind int

const (
	confirmNone confirmKind = iota
	confirmCloseAll
	confirmDeleteServer
)

type connStateFilter int

const (
	connActive connStateFilter = iota
	connAll
	connClosed
)

func (f connStateFilter) String() string {
	switch f {
	case connAll:
		return "all"
	case connClosed:
		return "closed"
	default:
		return "active"
	}
}

type Model struct {
	cfgPath    string
	cfg        *config.Config
	manager    *client.Manager
	session    *client.Session
	snapshot   client.Snapshot
	page       Page
	width      int
	height     int
	keys       keyMap
	mode       inputMode
	statusMsg  string
	statusAt   time.Time
	connectErr string

	pendingKey   string
	pendingKeyAt time.Time

	groupsCursor     int
	groupsItemCursor map[int]int
	groupsOffset     int
	expandOverride   map[string]bool
	pendingSelect    map[string]string
	testingTag       string
	pendingMode      string

	connCursor int
	connOffset int
	connSearch textinput.Model
	connFilter connStateFilter

	logTop         int
	logFollow      bool
	logLevelFilter int32
	logUseDefault  bool

	settingsCursor int
	settingsOffset int
	form           serverForm
	deleteIndex    int
	confirm        confirmKind
	returnMode     inputMode
}

func NewModel(cfgPath string, cfg *config.Config) Model {
	connSearch := textinput.New()
	connSearch.Prompt = "/"
	connSearch.Placeholder = "search"
	connSearch.CharLimit = 120
	return Model{
		cfgPath:          cfgPath,
		cfg:              cfg,
		manager:          client.NewManager(),
		keys:             defaultKeyMap(),
		groupsItemCursor: make(map[int]int),
		expandOverride:   make(map[string]bool),
		pendingSelect:    make(map[string]string),
		connSearch:       connSearch,
		logFollow:        true,
		logUseDefault:    true,
		logLevelFilter:   -1,
		mode:             modeNormal,
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), waitSessionUpdate(m.manager)}
	if m.cfg.ActiveServer() != nil {
		cmds = append(cmds, m.connectCmd())
	}
	return tea.Batch(cmds...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func waitSessionUpdate(mgr *client.Manager) tea.Cmd {
	return func() tea.Msg {
		s := mgr.Session()
		if s == nil {
			time.Sleep(200 * time.Millisecond)
			return waitSessionUpdate(mgr)()
		}
		u, ok := <-s.Updates()
		if !ok {
			time.Sleep(200 * time.Millisecond)
			return waitSessionUpdate(mgr)()
		}
		return sessionUpdateMsg(u)
	}
}

func (m Model) connectCmd() tea.Cmd {
	return func() tea.Msg {
		server := m.cfg.ActiveServer()
		if server == nil {
			return connectResultMsg{err: errNoServer}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return connectResultMsg{err: m.manager.Connect(ctx, server)}
	}
}

func (m Model) reconnectCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if m.manager.Session() == nil {
			server := m.cfg.ActiveServer()
			if server == nil {
				return connectResultMsg{err: errNoServer}
			}
			return connectResultMsg{err: m.manager.Connect(ctx, server)}
		}
		return connectResultMsg{err: m.manager.Reconnect(ctx)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.connSearch.Width = max(8, msg.Width-2)
		m.clampWindows()
		return m, nil

	case sessionUpdateMsg:
		m.snapshot = client.Snapshot(msg.Snapshot)
		m.session = m.manager.Session()
		m.reconcilePending()
		if m.snapshot.Connected {
			m.connectErr = ""
		} else if m.snapshot.ConnectError != "" {
			m.connectErr = ui.CleanConnectError(m.snapshot.ConnectError)
		}
		if m.logFollow {
			m.logTop = 0
		} else if m.logTop > len(m.filteredLogs()) {
			m.logFollow = true
			m.logTop = 0
		}
		m.clampWindows()
		return m, waitSessionUpdate(m.manager)

	case connectResultMsg:
		m.session = m.manager.Session()
		if msg.err != nil {
			m.connectErr = ui.CleanConnectError(client.ClassifyError(msg.err).Error())
		} else {
			m.connectErr = ""
			m.setStatus("connected")
		}
		return m, waitSessionUpdate(m.manager)

	case actionResultMsg:
		m.applyActionResult(msg)
		return m, nil

	case tickMsg:
		m.expirePendingKey()
		if m.statusMsg != "" && time.Since(m.statusAt) > statusTTL {
			m.statusMsg = ""
		}
		return m, tickCmd()

	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.expirePendingKey()

	switch m.mode {
	case modeSearch:
		return m.updateSearch(msg)
	case modeServerForm:
		return m.updateForm(msg)
	case modeHelp:
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) {
			next := m.returnMode
			if next == modeHelp {
				next = modeNormal
			}
			m.mode = next
			m.returnMode = modeNormal
		}
		return m, nil
	case modeConfirm:
		return m.updateConfirm(msg)
	case modeServers:
		return m.updateServers(msg)
	}

	if len(m.cfg.Servers) == 0 {
		return m.updateSetup(msg)
	}

	action := m.resolveKey(msg)
	if action == "" {
		return m, nil
	}

	if cmd, handled := m.handleGlobalKey(msg); handled {
		return m, cmd
	}
	return m.handlePageKey(msg, action)
}

func (m *Model) handleGlobalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit, true
	case key.Matches(msg, m.keys.Help):
		m.returnMode = modeNormal
		m.mode = modeHelp
		return nil, true
	case key.Matches(msg, m.keys.Reconnect):
		m.setStatus("reconnecting")
		return m.reconnectCmd(), true
	case key.Matches(msg, m.keys.Servers):
		m.mode = modeServers
		return nil, true
	case key.Matches(msg, m.keys.Groups):
		m.page = PageGroups
		return nil, true
	case key.Matches(msg, m.keys.Conns):
		m.page = PageConnections
		return nil, true
	case key.Matches(msg, m.keys.Logs):
		m.page = PageLogs
		return nil, true
	case key.Matches(msg, m.keys.NextTab):
		m.page = Page((int(m.page) + 1) % 3)
		return nil, true
	case key.Matches(msg, m.keys.PrevTab):
		m.page = Page((int(m.page) + 2) % 3)
		return nil, true
	case key.Matches(msg, m.keys.ModePrev):
		return m.cycleClash(-1), true
	case key.Matches(msg, m.keys.ModeNext):
		return m.cycleClash(1), true
	}
	return nil, false
}

func (m Model) handlePageKey(msg tea.KeyMsg, action string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.page {
	case PageGroups:
		cmd = m.updateGroupsKey(msg, action)
	case PageConnections:
		cmd = m.updateConnectionsKey(msg, action)
	case PageLogs:
		cmd = m.updateLogsKey(msg, action)
	}
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if len(m.cfg.Servers) == 0 && m.mode != modeServerForm && m.mode != modeHelp {
		return m.viewSetup()
	}
	if len(m.cfg.Servers) == 0 && (m.mode == modeServerForm || m.mode == modeHelp) {
		h := max(1, m.height-1)
		var body string
		if m.mode == modeServerForm {
			body = m.viewServerForm(h)
		} else {
			body = m.viewHelp(h)
		}
		return lipgloss.NewStyle().Width(m.width).Height(h).Render(body) + "\n" + m.renderStatusBar()
	}

	h := m.contentHeight()
	content := m.renderContent(h)
	var b string
	b += m.renderTabs() + "\n"
	b += lipgloss.NewStyle().Width(m.width).Height(h).MaxHeight(h).Render(content)
	b += "\n" + m.renderStatusBar()
	return b
}

func (m Model) contentHeight() int {
	h := m.height - 2
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) renderContent(h int) string {
	switch m.mode {
	case modeHelp:
		return m.viewHelp(h)
	case modeServers:
		return m.viewServerPicker(h)
	case modeServerForm:
		return m.viewServerForm(h)
	case modeConfirm:
		return m.viewConfirm(h)
	}
	switch m.page {
	case PageConnections:
		return m.viewConnections(h)
	case PageLogs:
		return m.viewLogs(h)
	default:
		return m.viewGroups(h)
	}
}

func (m *Model) setStatus(s string) {
	m.statusMsg = s
	m.statusAt = time.Now()
}

func (m *Model) connected() bool {
	return m.snapshot.Connected
}

func (m Model) usingClash() bool {
	return m.snapshot.API == "clash"
}

func (m *Model) clampWindows() {
	h := m.contentHeight()
	if rows := m.buildGroupRows(); len(rows) > 0 {
		flat := m.groupsFlatCursor()
		_, _, m.groupsOffset = visibleWindow(len(rows), flat, m.groupsOffset, h)
	} else {
		m.groupsOffset = 0
	}
	conns := m.filteredConnections()
	if len(conns) == 0 {
		m.connCursor = 0
		m.connOffset = 0
	} else {
		m.connCursor = clamp(m.connCursor, 0, len(conns)-1)
		lay := m.connLayout(h, conns)
		_, _, m.connOffset = visibleWindow(len(conns), m.connCursor, m.connOffset, lay.listH)
	}
	if n := len(m.cfg.Servers); n == 0 {
		m.settingsCursor = 0
		m.settingsOffset = 0
	} else {
		m.settingsCursor = clamp(m.settingsCursor, 0, n-1)
		_, _, m.settingsOffset = visibleWindow(n, m.settingsCursor, m.settingsOffset, h)
	}
	if n := len(m.snapshot.Groups); n == 0 {
		m.groupsCursor = 0
	} else {
		m.groupsCursor = clamp(m.groupsCursor, 0, n-1)
	}
}

func (m *Model) reconcilePending() {
	if m.pendingMode != "" && m.snapshot.ClashMode == m.pendingMode {
		m.pendingMode = ""
	}
	for tag, want := range m.pendingSelect {
		for _, g := range m.snapshot.Groups {
			if g.Tag == tag && g.Selected == want {
				delete(m.pendingSelect, tag)
			}
		}
	}
	for tag, want := range m.expandOverride {
		for _, g := range m.snapshot.Groups {
			if g.Tag == tag && g.IsExpand == want {
				delete(m.expandOverride, tag)
			}
		}
	}
}

func (m *Model) applyActionResult(msg actionResultMsg) {
	switch msg.kind {
	case "clash":
		if msg.err != nil {
			m.pendingMode = ""
			m.setStatus(msg.err.Error())
		}
	case "select":
		if msg.err != nil {
			delete(m.pendingSelect, msg.tag)
			m.setStatus(selectErrorHint(msg.err, m.usingClash()))
		}
	case "urltest":
		m.testingTag = ""
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		} else {
			m.setStatus("tested " + msg.tag)
		}
	case "clearLogs":
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		} else if m.usingClash() {
			m.setStatus("cleared locally — Clash API cannot clear server logs")
		}
	case "expand":
		if msg.err != nil {
			delete(m.expandOverride, msg.tag)
			m.setStatus(msg.err.Error())
		}
	case "close", "closeAll":
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		}
	}
}

func (m *Model) cycleClash(delta int) tea.Cmd {
	list := m.snapshot.ClashModeList
	if len(list) == 0 || m.session == nil {
		return nil
	}
	cur := m.snapshot.ClashMode
	if m.pendingMode != "" {
		cur = m.pendingMode
	}
	idx := 0
	for i, mode := range list {
		if mode == cur {
			idx = i
			break
		}
	}
	n := len(list)
	next := list[(idx+delta%n+n)%n]
	m.pendingMode = next
	sess := m.session
	return func() tea.Msg {
		return actionResultMsg{kind: "clash", tag: next, err: sess.SetClashMode(next)}
	}
}

func (m *Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Add):
		m.openForm()
	case key.Matches(msg, m.keys.Help):
		m.mode = modeHelp
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}
