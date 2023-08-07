package tui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var BaseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.HiddenBorder()).
	Background(lipgloss.Color("#3B4252"))

func NewTable(cols []table.Column, opts ...table.Option) table.Model {
	t := table.New(append([]table.Option{
		table.WithColumns(cols),
		table.WithFocused(true),
	}, opts...)...)
	s := table.DefaultStyles()
	s.Header = s.Header.
		Background(lipgloss.Color("#5e81ac")).
		Bold(true)
	s.Selected = lipgloss.NewStyle().Background(lipgloss.Color("#4C566A"))
	t.SetStyles(s)
	return t
}
