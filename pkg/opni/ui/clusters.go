package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.HiddenBorder()).
	Background(lipgloss.Color("#3B4252"))

type Ref[T any] struct {
	V *T
	R *corev1.Reference
}

type (
	HealthStatusUpdateEvent = Ref[corev1.HealthStatus]
)

type keymap struct {
	table.KeyMap
	Quit key.Binding
}

func (km keymap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.Quit,
		km.LineUp,
		km.LineDown,
		km.GotoTop,
		km.GotoBottom,
	}
}

func (km keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			km.Quit,
			km.LineUp,
			km.LineDown,
			km.GotoTop,
			km.GotoBottom,
		},
		{
			km.PageUp,
			km.PageDown,
			km.HalfPageUp,
			km.HalfPageDown,
		},
	}
}

type clusterData struct {
	cluster      *corev1.Cluster
	healthStatus *corev1.HealthStatus
}

type ClusterListModel struct {
	rows   []clusterData
	t      table.Model
	help   help.Model
	keymap help.KeyMap
	width  int
}

func NewClusterListModel() ClusterListModel {
	t := table.New(
		table.WithColumns([]table.Column{
			{
				Title: "ID",
				Width: 36,
			},
			{
				Title: "LABELS",
				Width: 24,
			},
			{
				Title: "CAPABILITIES",
				Width: 16,
			},
			{
				Title: "STATUS",
				Width: 16,
			},
		}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		Background(lipgloss.Color("#5e81ac")).
		Bold(true)
	s.Selected = lipgloss.NewStyle().Background(lipgloss.Color("#4C566A"))
	t.SetStyles(s)

	return ClusterListModel{
		t:    t,
		help: help.New(),
		keymap: keymap{
			KeyMap: table.DefaultKeyMap(),
			Quit:   key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		},
	}
}

func (m ClusterListModel) Init() tea.Cmd {
	return nil
}

func (m ClusterListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.t.SetWidth(msg.Width)
		m.help.Width = msg.Width
	case *managementv1.WatchEvent:
		switch msg.GetType() {
		case managementv1.WatchEventType_Created:
			m.rows = append(m.rows, clusterData{
				cluster: msg.GetCluster(),
			})
		case managementv1.WatchEventType_Updated:
			for i, row := range m.rows {
				if row.cluster.Id == msg.GetCluster().GetId() {
					m.rows[i].cluster = msg.GetCluster()
					break
				}
			}
		case managementv1.WatchEventType_Deleted:
			ref := msg.GetCluster()
			for i, row := range m.rows {
				if row.cluster.Id == ref.GetId() {
					m.rows = append(m.rows[:i], m.rows[i+1:]...)
					break
				}
			}
		}
	case HealthStatusUpdateEvent:
		if msg.R == nil || msg.V == nil {
			return m, nil
		}
		for i, r := range m.rows {
			if r.cluster.Id == msg.R.GetId() {
				if r.healthStatus != nil && r.healthStatus.Health != nil {
					if r.healthStatus.Health.NewerThan(msg.V.Health) {
						continue
					}
				}
				m.rows[i].healthStatus = msg.V
				break
			}
		}
	}

	var rows []table.Row
	for _, t := range m.rows {
		labels := []string{}
		for k, v := range t.cluster.GetMetadata().GetLabels() {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(labels)
		capabilities := []string{}
		for _, c := range t.cluster.GetCapabilities() {
			if c.DeletionTimestamp == nil {
				capabilities = append(capabilities, c.Name)
			} else {
				capabilities = append(capabilities, fmt.Sprintf("%s (deleting)", c.Name))
			}
		}
		row := table.Row{t.cluster.GetId(), strings.Join(labels, ","), strings.Join(capabilities, ",")}
		if t.healthStatus != nil {
			row = append(row, t.healthStatus.Summary())
		} else {
			row = append(row, "(unknown)")
		}
		rows = append(rows, row)
	}
	m.t.SetRows(rows)

	var cmd tea.Cmd
	var cmds []tea.Cmd

	m.t, cmd = m.t.Update(msg)
	cmds = append(cmds, cmd)
	m.help, cmd = m.help.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m ClusterListModel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		baseStyle.Render(m.t.View()),
		lipgloss.NewStyle().Margin(0, 1, 0, 1).Faint(true).Render(fmt.Sprintf("%d/%d", m.t.Cursor(), len(m.rows))),
		lipgloss.NewStyle().Margin(0, 1, 0, 1).Render(m.help.View(m.keymap)),
	)
}

type ClusterListWatcher struct {
	Messages chan tea.Msg
	Client   managementv1.ManagementClient
}

func (w *ClusterListWatcher) Run(ctx context.Context) error {
	stream, err := w.Client.WatchClusters(ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		return err
	}
	cancelMap := map[string]context.CancelFunc{}
	for {
		msg, err := stream.Recv()
		if err != nil {
			w.Messages <- tea.Quit()
			return err
		}
		switch msg.GetType() {
		case managementv1.WatchEventType_Created:
			ref := msg.GetCluster().Reference()
			if ca, ok := cancelMap[ref.GetId()]; ok {
				ca()
				delete(cancelMap, ref.GetId())
			}
			ctx, ca := context.WithCancel(ctx)
			stream, err := w.Client.WatchClusterHealthStatus(ctx, ref)
			if err != nil {
				ca()
				w.Messages <- tea.Quit()
				return err
			}
			cancelMap[ref.GetId()] = ca
			go func() {
				for {
					msg, err := stream.Recv()
					if err != nil {
						w.Messages <- HealthStatusUpdateEvent{R: ref, V: nil}
						return
					}
					w.Messages <- HealthStatusUpdateEvent{R: ref, V: msg}
				}
			}()
		case managementv1.WatchEventType_Deleted:
			ref := msg.GetCluster()
			if ca, ok := cancelMap[ref.GetId()]; ok {
				ca()
				delete(cancelMap, ref.GetId())
			}
		}

		w.Messages <- msg
	}
}
