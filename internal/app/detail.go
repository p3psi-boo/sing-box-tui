package app

import (
	"fmt"
	"os"
	"strings"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/p3psi-boo/sing-box-tui/internal/ui"
	"github.com/rivo/uniseg"
)

type detailView struct {
	title, text        string
	offset, horizontal int
	wrap               bool
	back               inputMode
}

func (m *Model) openDetail(title, text string) {
	m.detail = detailView{title: title, text: ansi.Strip(text), wrap: true, back: m.mode}
	m.mode = modeDetail
}

func (m Model) detailLines() []string {
	text := m.detail.text
	if m.detail.wrap {
		text = ansi.Hardwrap(text, max(1, m.width), true)
	}
	return strings.Split(text, "\n")
}

func (m Model) viewDetail(height int) string {
	lines := m.detailLines()
	h := max(1, height-1)
	top := clamp(m.detail.offset, 0, max(0, len(lines)-h))
	end := min(len(lines), top+h)
	head := fmt.Sprintf("%s · %d–%d/%d", m.detail.title, top+1, end, len(lines))
	out := []string{ui.Truncate(head, m.width)}
	for _, line := range lines[top:end] {
		if !m.detail.wrap {
			line = cutColumns(line, m.detail.horizontal, m.width)
		}
		out = append(out, ui.Truncate(line, m.width))
	}
	return strings.Join(out, "\n")
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = m.detail.back
	case "w":
		m.detail.wrap = !m.detail.wrap
		m.detail.offset = 0
		m.detail.horizontal = 0
	case "left", "h":
		m.detail.horizontal = max(0, m.detail.horizontal-8)
	case "right", "l":
		widest := 0
		for _, line := range strings.Split(m.detail.text, "\n") {
			widest = max(widest, ui.DisplayWidth(line))
		}
		m.detail.horizontal = min(max(0, widest-m.width), m.detail.horizontal+8)
	case "y":
		text := m.detail.text
		return m, func() tea.Msg {
			_, err := osc52.New(text).WriteTo(os.Stdout)
			return actionResultMsg{kind: "copy", err: err}
		}
	default:
		m.detail.offset = scrollKey(msg.String(), m.detail.offset, len(m.detailLines()), max(1, m.contentHeight()-1))
	}
	return m, nil
}

func scrollKey(k string, offset, total, height int) int {
	switch k {
	case "up", "k":
		offset--
	case "down", "j":
		offset++
	case "pgup", "ctrl+b":
		offset -= height
	case "pgdown", "ctrl+f":
		offset += height
	case "ctrl+u":
		offset -= max(1, height/2)
	case "ctrl+d":
		offset += max(1, height/2)
	case "home", "g":
		offset = 0
	case "end", "G":
		offset = total
	}
	return clamp(offset, 0, max(0, total-height))
}

func wrapText(text string, width int) string { return ansi.Hardwrap(text, max(1, width), true) }

func cutColumns(text string, start, width int) string {
	g := uniseg.NewGraphemes(text)
	col := 0
	var out strings.Builder
	for g.Next() {
		cluster := g.Str()
		w := uniseg.StringWidth(cluster)
		if col >= start && col+w <= start+width {
			out.WriteString(cluster)
		}
		col += w
		if col >= start+width {
			break
		}
	}
	return out.String()
}
