package tui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

var (
	BackgroundColor = lipgloss.CompleteColor{
		TrueColor: "#3B4252",
		ANSI256:   "59",
		ANSI:      "4",
	}
	HeaderColor = lipgloss.CompleteColor{
		TrueColor: "#5e81ac",
		ANSI256:   "67",
		ANSI:      "4",
	}
	SelectedColor = lipgloss.CompleteColor{
		TrueColor: "#4C566A",
		ANSI256:   "60",
		ANSI:      "4",
	}
	BorderOutlineColor = lipgloss.CompleteColor{
		TrueColor: "#8fbcbb",
		ANSI256:   "115",
		ANSI:      "6",
	}
)

var BaseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.HiddenBorder()).
	Background(BackgroundColor)
var SelectedStyle = table.DefaultStyles().Selected.Copy().
	Background(SelectedColor).
	Foreground(lipgloss.NoColor{})
var HeaderStyle = table.DefaultStyles().Header.Copy().
	Background(HeaderColor).
	Bold(true)

func NewTable(cols []table.Column, opts ...table.Option) table.Model {
	t := table.New(append([]table.Option{
		table.WithColumns(cols),
		table.WithFocused(true),
	}, opts...)...)
	s := table.DefaultStyles()
	s.Header = HeaderStyle
	s.Selected = SelectedStyle
	t.SetStyles(s)
	return t
}
