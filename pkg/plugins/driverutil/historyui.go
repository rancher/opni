package driverutil

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nsf/jsondiff"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/opni/ui"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type HistoryUI struct {
	model historyModel
}

type historyModel struct {
	entries       []entry
	table         table.Model
	selectedEntry *entry
	help          help.Model
	diffMode      string
}

type keymap struct {
	Quit        key.Binding
	LineUp      key.Binding
	LineDown    key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	GotoTop     key.Binding
	GotoBottom  key.Binding
	SwitchFocus key.Binding
	DiffView    key.Binding
}

var Keymap = keymap{
	Quit:        key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	LineUp:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	LineDown:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
	GotoTop:     key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "go to start")),
	GotoBottom:  key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "go to end")),
	SwitchFocus: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch focus")),
	DiffView:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle diff view")),
}

func (km keymap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.Quit,
		km.LineUp,
		km.LineDown,
		km.SwitchFocus,
		km.DiffView,
	}
}

func (km keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			km.Quit,
			km.LineUp,
			km.LineDown,
			km.PageUp,
			km.PageDown,
		},
		{
			km.SwitchFocus,
			km.DiffView,
		},
	}
}

func (m historyModel) Init() tea.Cmd {
	return nil
}

func (m historyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
		for i := range m.entries {
			m.entries[i].diffView.Width = msg.Width - m.table.Width() - 4
			m.entries[i].fullDiffView.Width = msg.Width - m.table.Width() - 4
			m.entries[i].jsonView.Width = msg.Width - m.table.Width() - 4
		}
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keymap.Quit):
			return m, tea.Quit
		case key.Matches(msg, Keymap.LineUp, Keymap.LineDown, Keymap.PageUp, Keymap.PageDown):
			if m.table.Focused() {
				m.table, cmd = m.table.Update(msg)
			} else {
				switch m.diffMode {
				case "diff":
					m.selectedEntry.diffView, cmd = m.selectedEntry.diffView.Update(msg)
				case "full-diff":
					m.selectedEntry.fullDiffView, cmd = m.selectedEntry.fullDiffView.Update(msg)
				case "none":
					m.selectedEntry.jsonView, cmd = m.selectedEntry.jsonView.Update(msg)
				}
			}
			cmds = append(cmds, cmd)
		case key.Matches(msg, Keymap.SwitchFocus):
			if m.table.Focused() {
				m.table.Blur()
			} else {
				m.table.Focus()
			}
		case key.Matches(msg, Keymap.DiffView):
			modes := []string{"diff", "full-diff", "none"}
			m.diffMode = modes[(slices.Index(modes, m.diffMode)+1)%len(modes)]
		case msg.String() == "?":
			m.help.ShowAll = !m.help.ShowAll
		}
	default:
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)

		m.selectedEntry.jsonView, cmd = m.selectedEntry.jsonView.Update(msg)
		cmds = append(cmds, cmd)

		m.selectedEntry.diffView, cmd = m.selectedEntry.diffView.Update(msg)
		cmds = append(cmds, cmd)

		m.selectedEntry.fullDiffView, cmd = m.selectedEntry.fullDiffView.Update(msg)
		cmds = append(cmds, cmd)
	}

	currentRow := m.table.Cursor()
	if m.selectedEntry != &m.entries[currentRow] {
		m.selectedEntry = &m.entries[currentRow]
	}

	return m, tea.Batch(cmds...)
}

func (m historyModel) View() string {
	left := m.renderTableView()
	right := m.renderJsonView()
	contents := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left,
		contents,
		lipgloss.NewStyle().Margin(0, 1, 0, 1).Render(m.help.View(Keymap)),
	)
}

func (m *historyModel) renderTableView() string {
	var border lipgloss.Border
	if m.table.Focused() {
		border = lipgloss.NormalBorder()
	} else {
		border = lipgloss.HiddenBorder()
	}
	return ui.BaseStyle.Copy().BorderStyle(border).BorderForeground(lipgloss.Color("#8fbcbb")).Render(m.table.View())
}

func (m *historyModel) renderJsonView() string {
	var model *viewport.Model
	var title string
	switch m.diffMode {
	case "diff":
		model = &m.selectedEntry.diffView
		title = "Diff View"
	case "full-diff":
		model = &m.selectedEntry.fullDiffView
		title = "Full Diff View"
	case "none":
		model = &m.selectedEntry.jsonView
		title = "JSON View"
	}
	style := lipgloss.NewStyle().Background(lipgloss.Color("#5e81ac")).Bold(true)
	line := fmt.Sprintf(" %s%s", title, strings.Repeat(" ", max(0, model.Width-lipgloss.Width(title)-1)))
	lineCount := model.TotalLineCount()
	scrollPercent := fmt.Sprintf("%d:%d/%d • %.f%%", model.YOffset, min(lineCount, model.YOffset+model.Height), lineCount, model.ScrollPercent()*100)

	footerLine := fmt.Sprintf(" %s%s", scrollPercent, strings.Repeat(" ", max(0, model.Width-lipgloss.Width(scrollPercent)-1)))

	renderedHeader := style.Render(line)
	renderedContent := model.View()
	renderedFooter := lipgloss.NewStyle().Background(lipgloss.Color("#3B4252")).Inline(true).Render(footerLine)

	var border lipgloss.Border
	if m.table.Focused() {
		border = lipgloss.HiddenBorder()
	} else {
		border = lipgloss.NormalBorder()
	}

	return lipgloss.NewStyle().BorderStyle(border).BorderForeground(lipgloss.Color("#8fbcbb")).Render(
		fmt.Sprintf("%s\n%s\n%s", renderedHeader, renderedContent, renderedFooter),
	)
}

func NewHistoryUI[T config_type[T]](ts []T) *HistoryUI {
	var entries []entry
	for i, e := range ts {
		entry := entry{
			revision: e.GetRevision(),
			cfg:      e,
		}
		bytes, _ := protojson.MarshalOptions{
			EmitUnpopulated: true,
			UseProtoNames:   true,
			Multiline:       true,
			Indent:          "  ",
		}.Marshal(ts[i])

		jsonView := viewport.New(0, 29)
		jsonView.SetContent(string(bytes))
		entry.jsonView = jsonView

		if i > 0 {
			prev := entries[i-1]
			opts := jsondiff.DefaultConsoleOptions()
			opts.SkipMatches = true
			str, _ := RenderJsonDiff(prev.cfg, entry.cfg, opts)

			diffView := viewport.New(0, 29)
			diffView.SetContent(str)

			entry.diffView = diffView
			entry.diffSummary = diffSummary(str)

			opts.SkipMatches = false
			str, _ = RenderJsonDiff(prev.cfg, entry.cfg, opts)
			fullDiffView := viewport.New(0, 29)
			fullDiffView.SetContent(str)
			entry.fullDiffView = fullDiffView
		}

		entries = append(entries, entry)
	}
	columns := []table.Column{
		{Title: "Rev", Width: 5},
		{Title: "Diff", Width: 10},
		{Title: "Timestamp", Width: 17},
	}

	rows := make([]table.Row, len(entries))
	for i := range entries {
		entry := entries[i]
		rows[i] = table.Row{
			fmt.Sprint(entry.revision.GetRevision()),
			entry.diffSummary,
			entry.revision.GetTimestamp().AsTime().Format("06-01-02 15:04:05"),
		}
	}

	table := ui.NewTable(columns, table.WithRows(rows), table.WithHeight(30), table.WithWidth(38))
	table.GotoBottom()
	help := help.New()
	return &HistoryUI{
		model: historyModel{
			entries:       entries,
			table:         table,
			help:          help,
			selectedEntry: &entries[len(entries)-1],
			diffMode:      "diff",
		},
	}
}

func (ui *HistoryUI) Run() error {
	p := tea.NewProgram(ui.model)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

type entry struct {
	cfg          proto.Message
	cfgJson      string
	revision     *corev1.Revision
	diffSummary  string
	jsonView     viewport.Model
	diffView     viewport.Model
	fullDiffView viewport.Model
}

func diffSummary(diff string) string {
	if diff == "" {
		return ""
	}
	numAdded := strings.Count(diff, "\033[0;32m")
	numRemoved := strings.Count(diff, "\033[0;31m")
	numChanged := strings.Count(diff, "\033[0;33m")

	parts := []string{}
	if numAdded > 0 {
		parts = append(parts, fmt.Sprintf("+%d", numAdded))
	}
	if numRemoved > 0 {
		parts = append(parts, fmt.Sprintf("-%d", numRemoved))
	}
	if numChanged > 0 {
		parts = append(parts, fmt.Sprintf("~%d", numChanged))
	}

	return strings.Join(parts, "/")
}
