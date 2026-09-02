package app

import (
	"strings"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type GroupRow struct {
	GroupIndex int
	ItemIndex  int
	IsHeader   bool
}

func (m Model) groupExpanded(g *daemon.Group) bool {
	if g == nil {
		return false
	}
	if v, ok := m.expandOverride[g.Tag]; ok {
		return v
	}
	return g.IsExpand
}

func (m Model) groupSelected(g *daemon.Group) string {
	if g == nil {
		return ""
	}
	if tag, ok := m.pendingSelect[g.Tag]; ok {
		return tag
	}
	return g.Selected
}

func (m Model) buildGroupRows() []GroupRow {
	var rows []GroupRow
	for i, group := range m.snapshot.Groups {
		rows = append(rows, GroupRow{GroupIndex: i, ItemIndex: -1, IsHeader: true})
		if m.groupExpanded(group) {
			for j := range group.Items {
				rows = append(rows, GroupRow{GroupIndex: i, ItemIndex: j, IsHeader: false})
			}
		}
	}
	return rows
}

func (m *Model) syncGroupsCursorFromFlat(flat int) {
	rows := m.buildGroupRows()
	if flat < 0 || flat >= len(rows) {
		return
	}
	row := rows[flat]
	m.groupsCursor = row.GroupIndex
	if row.IsHeader {
		m.groupsItemCursor[row.GroupIndex] = -1
	} else {
		m.groupsItemCursor[row.GroupIndex] = row.ItemIndex
	}
}

func (m Model) groupsFlatCursor() int {
	rows := m.buildGroupRows()
	itemIdx := -1
	if v, ok := m.groupsItemCursor[m.groupsCursor]; ok {
		itemIdx = v
	}
	header := 0
	for i, row := range rows {
		if row.GroupIndex != m.groupsCursor {
			continue
		}
		if row.IsHeader {
			header = i
			if itemIdx < 0 {
				return i
			}
		} else if row.ItemIndex == itemIdx {
			return i
		}
	}
	return header
}

func (m Model) viewGroups(height int) string {
	if !m.connected() {
		return m.viewDisconnected("")
	}
	if !m.snapshot.GroupsLoaded {
		return "Waiting for groups"
	}
	if len(m.snapshot.Groups) == 0 {
		return "No proxy groups"
	}

	rows := m.buildGroupRows()
	flat := m.groupsFlatCursor()
	start, end, _ := visibleGroups(rows, flat, m.groupsOffset, height)

	var lines []string
	for i := start; i < end; i++ {
		if i > start && rows[i].IsHeader {
			lines = append(lines, ui.Divider(m.width))
		}
		row := rows[i]
		group := m.snapshot.Groups[row.GroupIndex]
		cur := i == flat
		if row.IsHeader {
			lines = append(lines, m.renderGroupHeader(group, cur))
			continue
		}
		item := group.Items[row.ItemIndex]
		lines = append(lines, m.renderGroupItem(group, item, cur))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderGroupHeader(group *daemon.Group, current bool) string {
	prefix := "  "
	if current {
		prefix = "> "
	}
	selected := m.groupSelected(group)
	left := prefix + group.Tag
	if selected != "" {
		left += "  → " + selected
	}
	var rightParts []string
	if m.testingTag == group.Tag {
		rightParts = append(rightParts, "…")
	} else if delay := selectedDelay(group, selected); delay > 0 {
		rightParts = append(rightParts, ui.DelayStyle(delay).Render(ui.FormatDelay(delay)))
	}
	right := strings.Join(rightParts, " ")
	line := ui.FitRow(left, right, m.width)
	if current {
		return ui.SelectedStyle.Render(line)
	}
	return line
}

func (m Model) renderGroupItem(group *daemon.Group, item *daemon.GroupItem, current bool) string {
	prefix := "    "
	if current {
		prefix = "  ▸ "
	}
	left := prefix + item.Tag
	if item.Type != "" {
		left += "  " + ui.DimStyle.Render(ui.ProxyTypeLabel(item.Type))
	}
	var rightParts []string
	if item.UrlTestDelay > 0 {
		rightParts = append(rightParts, ui.DelayStyle(item.UrlTestDelay).Render(ui.FormatDelay(item.UrlTestDelay)))
	}
	if m.groupSelected(group) == item.Tag {
		rightParts = append(rightParts, ui.GoodStyle.Render("✓"))
	}
	line := ui.FitRow(left, strings.Join(rightParts, "  "), m.width)
	if current {
		return ui.SelectedStyle.Render(line)
	}
	return line
}

func selectedDelay(group *daemon.Group, selected string) int32 {
	if selected == "" {
		return 0
	}
	for _, item := range group.Items {
		if item.Tag == selected {
			return item.UrlTestDelay
		}
	}
	return 0
}

func (m *Model) updateGroupsKey(msg tea.KeyMsg, action string) tea.Cmd {
	if !m.connected() || len(m.snapshot.Groups) == 0 {
		return nil
	}
	rows := m.buildGroupRows()
	if len(rows) == 0 {
		return nil
	}
	flat := m.groupsFlatCursor()
	h := m.contentHeight()

	move := func(next int) {
		m.syncGroupsCursorFromFlat(clamp(next, 0, len(rows)-1))
		_, _, m.groupsOffset = visibleGroups(rows, m.groupsFlatCursor(), m.groupsOffset, h)
	}

	switch {
	case action == "gg" || key.Matches(msg, m.keys.Top):
		move(0)
	case action == "G" || key.Matches(msg, m.keys.Bottom):
		move(len(rows) - 1)
	case m.isDown(msg):
		move(flat + 1)
	case m.isUp(msg):
		move(flat - 1)
	case key.Matches(msg, m.keys.PageDown):
		move(flat + pageStep(h))
	case key.Matches(msg, m.keys.PageUp):
		move(flat - pageStep(h))
	case key.Matches(msg, m.keys.HalfDown):
		move(flat + halfPageStep(h))
	case key.Matches(msg, m.keys.HalfUp):
		move(flat - halfPageStep(h))
	case key.Matches(msg, m.keys.Expand):
		return m.toggleExpand()
	case key.Matches(msg, m.keys.Test):
		return m.urlTestCurrent()
	case key.Matches(msg, m.keys.Confirm):
		return m.activateGroupRow(rows[clamp(flat, 0, len(rows)-1)])
	}
	return nil
}

func (m *Model) toggleExpand() tea.Cmd {
	if m.groupsCursor < 0 || m.groupsCursor >= len(m.snapshot.Groups) {
		return nil
	}
	group := m.snapshot.Groups[m.groupsCursor]
	next := !m.groupExpanded(group)
	m.expandOverride[group.Tag] = next
	if !next {
		m.groupsItemCursor[m.groupsCursor] = -1
	}
	m.clampWindows()
	if m.session == nil {
		return nil
	}
	sess := m.session
	tag := group.Tag
	return func() tea.Msg {
		return actionResultMsg{kind: "expand", tag: tag, err: sess.SetGroupExpand(tag, next)}
	}
}

func (m *Model) urlTestCurrent() tea.Cmd {
	if m.groupsCursor < 0 || m.groupsCursor >= len(m.snapshot.Groups) || m.session == nil {
		return nil
	}
	tag := m.snapshot.Groups[m.groupsCursor].Tag
	m.testingTag = tag
	m.setStatus("testing " + tag)
	sess := m.session
	return func() tea.Msg {
		return actionResultMsg{kind: "urltest", tag: tag, err: sess.URLTest(tag)}
	}
}

func (m *Model) activateGroupRow(row GroupRow) tea.Cmd {
	group := m.snapshot.Groups[row.GroupIndex]
	if row.IsHeader {
		return m.toggleExpand()
	}
	if m.session == nil {
		return nil
	}
	if !group.Selectable {
		if m.usingClash() {
			m.setStatus("Clash API can only switch Selector groups")
		} else {
			m.setStatus("this group picks its own node")
		}
		return nil
	}
	item := group.Items[row.ItemIndex]
	m.pendingSelect[group.Tag] = item.Tag
	m.setStatus("selected " + item.Tag)
	sess := m.session
	gTag, iTag := group.Tag, item.Tag
	return func() tea.Msg {
		return actionResultMsg{kind: "select", tag: gTag, err: sess.SelectOutbound(gTag, iTag)}
	}
}

func selectErrorHint(err error, clash bool) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	if clash && (strings.Contains(lower, "selector") || strings.Contains(lower, "must be")) {
		return "Clash API can only switch Selector groups"
	}
	return msg
}
