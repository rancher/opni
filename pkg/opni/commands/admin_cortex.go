//go:build ignore

package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildCortexClusterStatusCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Cortex cluster status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := opsClient.GetClusterStatus(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if follow {
				switch status.State {
				case cortexops.InstallState_Updating:
					return watchForDesiredState(cortexops.InstallState_Installed)
				case cortexops.InstallState_Uninstalling:
					return watchForDesiredState(cortexops.InstallState_NotInstalled)
				}
				status, err = opsClient.GetClusterStatus(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
			}

			switch status.State {
			case cortexops.InstallState_NotInstalled:
				fmt.Println(chalk.Red.Color("Not Installed"))
				return nil
			case cortexops.InstallState_Updating:
				fmt.Println(chalk.Yellow.Color("Updating"))
			case cortexops.InstallState_Installed:
				fmt.Println(chalk.Green.Color("Installed"))
			case cortexops.InstallState_Uninstalling:
				fmt.Println(chalk.Yellow.Color("Uninstalling"))
				return nil
			case cortexops.InstallState_Unknown:
				fmt.Println("Unknown")
				return nil
			}

			fmt.Printf("Version: %s\n", status.Version)
			for k, v := range status.Metadata {
				fmt.Printf("%s: %s\n", k, v)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&follow, "follow", false, "Follow status updates")
	return cmd
}

func BuildCortexClusterConfigureCmd() *cobra.Command {
	var mode string
	var storage storagev1.StorageSpec
	var grafanaEnabled bool
	var grafanaHostname string
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Install or configure a Cortex cluster",
		Long: `Install or configure a Cortex cluster. If the cluster is already installed, this command will update the cluster configuration.
Some fields contain secrets. You may provide the placeholder value ` + chalk.Red.Color("***") + ` to keep an existing secret when updating the cluster configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			modeValue, ok := cortexops.DeploymentMode_value[mode]
			if !ok {
				return fmt.Errorf("unknown mode %q", mode)
			}
			if storage.Backend == "swift" {
				storage.Swift.LoadFromEnv()
			}
			_, err := opsClient.ConfigureCluster(cmd.Context(), &cortexops.ClusterConfiguration{
				Mode:    cortexops.DeploymentMode(modeValue),
				Storage: &storage,
				Grafana: &cortexops.GrafanaConfig{
					Enabled:  &grafanaEnabled,
					Hostname: grafanaHostname,
				},
			})
			if err != nil {
				return err
			}
			lg.With(
				"mode", mode,
				"storage", storage.Backend,
			).Info("Configuration applied")

			return watchForDesiredState(cortexops.InstallState_Updating, cortexops.InstallState_Installed)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Deployment mode (one of: AllInOne, HighlyAvailable)")
	cmd.Flags().AddFlagSet(storage.FlagSet())
	cmd.Flags().BoolVar(&grafanaEnabled, "grafana", true, "Enable Grafana")
	cmd.Flags().StringVar(&grafanaHostname, "grafana-hostname", "", "Grafana hostname")
	cmd.MarkFlagRequired("mode")
	cmd.MarkFlagRequired("storage.backend")
	return cmd
}

func BuildCortexClusterGetConfigurationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get-configuration",
		Aliases: []string{"get-config"},
		Short:   "Get the current Cortex cluster configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := opsClient.GetClusterConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			fmt.Println(protojson.MarshalOptions{
				Multiline:       true,
				EmitUnpopulated: true,
			}.Format(config))
			return nil
		},
	}
	return cmd
}

func BuildCortexClusterUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall a Cortex cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := opsClient.UninstallCluster(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			return watchForDesiredState(cortexops.InstallState_NotInstalled)
		},
	}
	return cmd
}

func watchForDesiredState(desiredStates ...cortexops.InstallState) error {
	m := clusterStatusModel{
		desiredStates: desiredStates,
		spinner:       spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle)),
	}
	lm, err := tea.NewProgram(m).StartReturningModel()
	if err != nil {
		return err
	}
	if err := lm.(clusterStatusModel).err; err != nil {
		return err
	}
	return nil
}

var (
	helpStyle      = lipgloss.NewStyle().Faint(true)
	conditionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	spinnerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
)

type tickMsg time.Time

type clusterStatusModel struct {
	desiredStates []cortexops.InstallState
	spinner       spinner.Model
	status        *cortexops.InstallStatus
	quitting      bool
	err           error
}

func (m clusterStatusModel) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.spinner.Tick)
}

func (m clusterStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		default:
			return m, nil
		}
	case tickMsg:
		if m.status.GetState() == m.desiredStates[0] {
			if len(m.desiredStates) > 1 {
				m.desiredStates = m.desiredStates[1:]
			} else {
				if !m.quitting {
					m.quitting = true
				} else {
					return m, tea.Quit
				}
			}
		}
		status, err := opsClient.GetClusterStatus(context.Background(), &emptypb.Empty{})
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.status = status
		return m, tickCmd()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m clusterStatusModel) View() (s string) {
	if m.status == nil {
		return
	}
	if m.quitting {
		return
	}
	s += fmt.Sprintf("\n %s%s (waiting for state: %s)\n", m.spinner.View(), m.status.State.String(), m.desiredStates[0].String())
	if conditions, ok := m.status.Metadata["Conditions"]; ok {
		list := lo.Map(strings.Split(conditions, ";"), util.Indexed(strings.TrimSpace))
		sort.Strings(list)
		for _, condition := range list {
			if strings.TrimSpace(condition) == "" {
				continue
			}
			s += fmt.Sprintf(" • %s\n", conditionStyle.Render(condition))
		}
	}
	s += helpStyle.Render("\n q: exit")
	return
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
