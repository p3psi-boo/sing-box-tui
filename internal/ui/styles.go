package ui

import "github.com/charmbracelet/lipgloss"

var (
	TabActive     = lipgloss.NewStyle().Bold(true).Reverse(true)
	TabInactive   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	ErrorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	WarningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	GoodStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	MediumStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	BadStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	SelectedStyle = lipgloss.NewStyle().Bold(true)
	DimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	StatusOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	StatusFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	StatusMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func DelayStyle(ms int32) lipgloss.Style {
	if ms <= 0 {
		return DimStyle
	}
	if ms < 800 {
		return GoodStyle
	}
	if ms < 1500 {
		return MediumStyle
	}
	return BadStyle
}
