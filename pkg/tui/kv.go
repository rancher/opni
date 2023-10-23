package tui

import (
	"sort"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type KeyValueStoreUI[T proto.Message] struct {
	model keyValueStoreModel[T]
}

type keyValueStoreKeymap struct {
	Quit       key.Binding
	LineUp     key.Binding
	LineDown   key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	GotoTop    key.Binding
	GotoBottom key.Binding
}

var KeyValueStoreKeymap = keyValueStoreKeymap{
	Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	LineUp:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	LineDown:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:     key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	PageDown:   key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
	GotoTop:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "go to start")),
	GotoBottom: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "go to end")),
}

func (km keyValueStoreKeymap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.Quit,
		km.LineUp,
		km.LineDown,
	}
}

func (km keyValueStoreKeymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			km.Quit,
			km.LineUp,
			km.LineDown,
			km.PageUp,
			km.PageDown,
		},
	}
}

func NewKeyValueStoreUI[T proto.Message](events <-chan storage.WatchEvent[storage.KeyRevision[[]byte]]) *KeyValueStoreUI[T] {
	return &KeyValueStoreUI[T]{
		model: keyValueStoreModel[T]{
			table: NewTable([]table.Column{
				{Title: "Key"},
				{Title: "Revision"},
				{Title: "Value"},
			}),
			help:   help.New(),
			kvs:    make(map[string]string),
			events: events,
		},
	}
}

type keyValueStoreModel[T proto.Message] struct {
	table  table.Model
	help   help.Model
	kvs    map[string]string
	events <-chan storage.WatchEvent[storage.KeyRevision[[]byte]]
	size   tea.WindowSizeMsg
}

// Init implements tea.Model.
func (m keyValueStoreModel[T]) Init() tea.Cmd {
	return nil
}

// View implements tea.Model.
func (m keyValueStoreModel[T]) View() string {
	return lipgloss.JoinVertical(lipgloss.Top, m.table.View(), m.help.View(KeyValueStoreKeymap))
}

func (m *keyValueStoreModel[T]) resizeColumns() {
	sizes := make([]int, 3)
	for _, row := range m.table.Rows() {
		for j, col := range row {
			sizes[j] = max(sizes[j], lipgloss.Width(col))
		}
	}
	sizes[0] = max(sizes[0], lipgloss.Width("Key"))
	sizes[1] = max(sizes[1], lipgloss.Width("Revision"))
	sizes[2] = m.size.Width - sizes[0] - sizes[1] - 4
	m.table.SetColumns([]table.Column{
		{Title: "Key", Width: sizes[0] + 1},
		{Title: "Revision", Width: sizes[1] + 1},
		{Title: "Value", Width: sizes[2] + 1},
	})
	m.help.Width = m.size.Width
}

func (m keyValueStoreModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// auto-resize the table
		m.size = msg
		m.resizeColumns()
	case storage.WatchEvent[storage.KeyRevision[[]byte]]:
		switch msg.EventType {
		case storage.WatchEventPut:
			t := util.NewMessage[T]()
			if err := proto.Unmarshal(msg.Current.Value(), t); err != nil {
				panic(err)
			}
			str, _ := protojson.Marshal(t)
			m.kvs[string(msg.Current.Key())] = string(str)
		case storage.WatchEventDelete:
			delete(m.kvs, string(msg.Previous.Key()))
		}
		rows := make([]table.Row, 0, len(m.kvs))
		for k, v := range m.kvs {
			rows = append(rows, table.Row{k, "-", v})
		}
		sort.Slice(rows, func(i, j int) bool {
			return rows[i][0] < rows[j][0]
		})
		m.table.SetRows(rows)
		m.resizeColumns()
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, KeyValueStoreKeymap.Quit):
			return m, tea.Quit
		case key.Matches(msg, KeyValueStoreKeymap.LineUp, KeyValueStoreKeymap.LineDown, KeyValueStoreKeymap.PageUp, KeyValueStoreKeymap.PageDown, KeyValueStoreKeymap.GotoTop, KeyValueStoreKeymap.GotoBottom):
			m.table, cmd = m.table.Update(msg)
			cmds = append(cmds, cmd)
		}
	default:
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (ui *KeyValueStoreUI[T]) Run() error {
	p := tea.NewProgram(ui.model)
	go func() {
		for event := range ui.model.events {
			p.Send(event)
		}
	}()
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
