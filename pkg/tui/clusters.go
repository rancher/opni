package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Ref[T any] struct {
	V *T
	R *corev1.Reference
}

type Keymap struct {
	table.KeyMap
	Quit key.Binding
}

func (km Keymap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.Quit,
		km.LineUp,
		km.LineDown,
		km.GotoTop,
		km.GotoBottom,
	}
}

func (km Keymap) FullHelp() [][]key.Binding {
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
	rows                 []clusterData
	lateJoinHealthStatus map[string]*corev1.HealthStatus
	t                    table.Model
	help                 help.Model
	keymap               help.KeyMap
	width                int
	columns              []table.Column
	showSessionAttrs     bool
}

func NewTableKeymap() Keymap {
	return Keymap{
		KeyMap: table.DefaultKeyMap(),
		Quit:   key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func NewClusterListModel() ClusterListModel {
	cols := []table.Column{
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
	}

	t := NewTable(cols, table.WithHeight(10))
	return ClusterListModel{
		t:                    t,
		help:                 help.New(),
		keymap:               NewTableKeymap(),
		columns:              cols,
		lateJoinHealthStatus: make(map[string]*corev1.HealthStatus),
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
			healthStatus, ok := m.lateJoinHealthStatus[msg.GetCluster().GetId()]
			if ok {
				delete(m.lateJoinHealthStatus, msg.GetCluster().GetId())
			}
			m.rows = append(m.rows, clusterData{
				cluster:      msg.GetCluster(),
				healthStatus: healthStatus,
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
	case *corev1.ClusterHealthStatus:
		if msg.Cluster == nil || msg.HealthStatus == nil {
			return m, nil
		}
		found := false
		for i, r := range m.rows {
			if r.cluster.Id == msg.Cluster.GetId() {
				if r.healthStatus != nil && r.healthStatus.Health != nil {
					if r.healthStatus.Health.NewerThan(msg.HealthStatus.Health) {
						continue
					}
				}
				m.rows[i].healthStatus = msg.HealthStatus
				found = true
				break
			}
		}
		if !found {
			m.lateJoinHealthStatus[msg.Cluster.GetId()] = msg.HealthStatus
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
			if !m.showSessionAttrs && len(t.healthStatus.GetStatus().GetSessionAttributes()) > 0 {
				m.showSessionAttrs = true
				m.t.SetColumns(append(m.columns, table.Column{
					Title: "ATTRIBUTES",
					Width: 24,
				}))
			}
			row = append(row, t.healthStatus.Summary())
			if m.showSessionAttrs {
				row = append(row, strings.Join(t.healthStatus.GetStatus().GetSessionAttributes(), ", "))
			}
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
		BaseStyle.Render(m.t.View()),
		lipgloss.NewStyle().Margin(0, 1, 0, 1).Faint(true).Render(fmt.Sprintf("%d/%d", m.t.Cursor(), len(m.rows))),
		lipgloss.NewStyle().Margin(0, 1, 0, 1).Render(m.help.View(m.keymap)),
	)
}

type ClusterListWatcher struct {
	Messages chan tea.Msg
	Client   managementv1.ManagementClient
}

func (w *ClusterListWatcher) Run(ctx context.Context) error {
	clusterStream, err := w.Client.WatchClusters(ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		return err
	}
	statusStream, err := w.Client.WatchClusterHealthStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}

	group, _ := errgroup.WithContext(ctx)
	group.Go(func() error {
		for {
			msg, err := clusterStream.Recv()
			if err != nil {
				w.Messages <- tea.Quit()
				return err
			}
			w.Messages <- msg
		}
	})
	group.Go(func() error {
		for {
			msg, err := statusStream.Recv()
			if err != nil {
				w.Messages <- tea.Quit()
				return err
			}
			w.Messages <- msg
		}
	})

	return group.Wait()
}
