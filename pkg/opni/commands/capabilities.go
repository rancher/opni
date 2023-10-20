//go:build !minimal

package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"slices"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/protobuf/types/known/emptypb"
)

var statusLog = logger.New(
	logger.WithDisableCaller(),
	logger.WithDisableTime(),
)

func BuildCapabilityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "capability",
		Aliases: []string{"cap", "capabilities"},
		Short:   "Manage cluster capabilities",
	}
	cmd.AddCommand(BuildCapabilityListCmd())
	cmd.AddCommand(BuildCapabilityInstallCmd())
	cmd.AddCommand(BuildCapabilityUninstallCmd())
	cmd.AddCommand(BuildCapabilityStatusCmd())
	cmd.AddCommand(BuildCapabilityCancelUninstallCmd())
	ConfigureManagementCommand(cmd)
	return cmd
}

func BuildCapabilityListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List available capabilities",
		Run: func(cmd *cobra.Command, args []string) {
			list, err := mgmtClient.ListCapabilities(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderCapabilityList(list))
		},
	}
	return cmd
}

func BuildCapabilityInstallCmd() *cobra.Command {
	var ignoreWarnings bool
	cmd := &cobra.Command{
		Use:   "install <capability-name> <cluster-id> [cluster-id ...]",
		Short: "Install a capability on one or more clusters",
		Args:  cobra.MinimumNArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeCapability(cmd, args, toComplete)
			}
			return completeClusters(cmd, args[1:], toComplete, filterDoesNotHaveCapability(args[0]))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, clusterID := range args[1:] {
				resp, err := mgmtClient.InstallCapability(cmd.Context(), &managementv1.CapabilityInstallRequest{
					Name: args[0],
					Target: &capabilityv1.InstallRequest{
						Cluster: &corev1.Reference{
							Id: clusterID,
						},
						IgnoreWarnings: ignoreWarnings,
					},
				})
				if err != nil {
					return err
				}

				switch resp.Status {
				case capabilityv1.InstallResponseStatus_Success:
					lg.Info("Capability installed successfully")
				case capabilityv1.InstallResponseStatus_Warning:
					lg.Warn("Capability installed with warning: " + resp.Message)
				case capabilityv1.InstallResponseStatus_Error:
					lg.Error("Capability installation failed (retry with --ignore-warnings to install anyway): " + resp.Message)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&ignoreWarnings, "ignore-warnings", false, "Proceed with installation even if warnings are present")
	return cmd
}

func BuildCapabilityUninstallCmd() *cobra.Command {
	var options capabilityv1.DefaultUninstallOptions
	var follow bool

	cmd := &cobra.Command{
		Use:   "uninstall <capability-name> <cluster-id> [cluster-id ...]",
		Short: "Uninstall a capability from one or more clusters",
		Args:  cobra.MinimumNArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeCapability(cmd, args, toComplete, filterNodeCountNonZero)
			}
			return completeClusters(cmd, args[1:], toComplete, filterHasCapability(args[0]))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, clusterID := range args[1:] {
				_, err := mgmtClient.UninstallCapability(cmd.Context(), &managementv1.CapabilityUninstallRequest{
					Name: args[0],
					Target: &capabilityv1.UninstallRequest{
						Cluster: &corev1.Reference{
							Id: clusterID,
						},
						Options: options.ToStruct(),
					},
				})
				if err != nil {
					return fmt.Errorf("uninstall failed: %w", err)
				}

				lg.Info("Uninstall request submitted successfully")
			}
			if !follow || len(args[1:]) > 1 {
				return nil
			}

			lg.Info("Watching for progress updates...")
			logTaskProgress(cmd.Context(), args[1], args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&options.DeleteStoredData, "delete-stored-data", false, "Delete all stored data associated with the capability")
	cmd.Flags().DurationVar((*time.Duration)(&options.InitialDelay), "initial-delay", 0, "Delay the uninstall operation by this amount of time, during which the operation can be canceled without incurring any data loss.")
	cmd.Flags().BoolVar(&follow, "follow", true, "follow progress of uninstall task")
	return cmd
}

func BuildCapabilityCancelUninstallCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "cancel-uninstall <capability-name> <cluster-id>",
		Short: "Cancel an in-progress uninstall of a capability",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeCapability(cmd, args, toComplete, filterNodeCountNonZero)
			}
			return completeClusters(cmd, args[1:], toComplete, filterHasCapability(args[0]), func(c *corev1.Cluster) bool {
				for _, cap := range c.GetCapabilities() {
					return cap.DeletionTimestamp != nil
				}
				return false
			})
		},
		Run: func(cmd *cobra.Command, args []string) {
			_, err := mgmtClient.CancelCapabilityUninstall(cmd.Context(), &managementv1.CapabilityUninstallCancelRequest{
				Name: args[0],
				Cluster: &corev1.Reference{
					Id: args[1],
				},
			})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			lg.Info("Cancel request submitted successfully")
			if !follow {
				return
			}
			lg.Info("Watching for progress updates...")
			logTaskProgress(cmd.Context(), args[1], args[0])
		},
	}
	cmd.Flags().BoolVar(&follow, "follow", true, "follow progress of uninstall task")
	return cmd
}

func BuildCapabilityStatusCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "status <capability-name> <cluster-id>",
		Short: "Show the status of a capability, or the status of an in-progress uninstall operation",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeCapability(cmd, args, toComplete, filterNodeCountNonZero)
			}
			return completeClusters(cmd, args[1:], toComplete, filterHasCapability(args[0]))
		},
		Run: func(cmd *cobra.Command, args []string) {
			cluster, err := mgmtClient.GetCluster(cmd.Context(), &corev1.Reference{
				Id: args[1],
			})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			for _, cap := range cluster.GetCapabilities() {
				if cap.Name != args[0] {
					continue
				}
				if cap.DeletionTimestamp != nil {
					fmt.Println(chalk.Yellow.Color("Uninstalling"))
					if follow {
						logTaskProgress(cmd.Context(), args[1], args[0])
					}
					return
				}
				stat, err := mgmtClient.CapabilityStatus(cmd.Context(), &managementv1.CapabilityStatusRequest{
					Name:    cap.Name,
					Cluster: cluster.Reference(),
				})
				fmt.Println(chalk.Green.Color("Installed"))
				if err != nil {
					fmt.Printf("Error looking up capability status: %s", chalk.Red.Color(err.Error()))
				} else {
					fmt.Printf("Enabled: %t\n", stat.Enabled)
					fmt.Printf("Last Sync: %s\n", stat.LastSync.AsTime().Format(time.RFC3339))
					if len(stat.Conditions) > 0 {
						fmt.Println("Conditions:")
						for _, cond := range stat.Conditions {
							fmt.Println("  " + chalk.Yellow.Color(cond))
						}
					}
				}
				return
			}
			fmt.Println(chalk.Red.Color("Not installed"))
		},
	}
	cmd.Flags().BoolVar(&follow, "follow", false, "follow progress of uninstall task")
	return cmd
}

func logTaskProgress(ctx context.Context, cluster, name string) error {
	lastLogTimestamp := time.Time{}
	for {
		status, err := mgmtClient.CapabilityUninstallStatus(ctx, &managementv1.CapabilityStatusRequest{
			Name: name,
			Cluster: &corev1.Reference{
				Id: cluster,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}
		allLogs := []corev1.TimestampedLog{}
		for _, log := range status.GetLogs() {
			allLogs = append(allLogs, log)
		}
		for _, tr := range status.GetTransitions() {
			allLogs = append(allLogs, tr)
		}
		slices.SortFunc(allLogs, func(a, b corev1.TimestampedLog) int {
			return a.GetTimestamp().AsTime().Compare(b.GetTimestamp().AsTime())
		})
		allLogs = lo.DropWhile(allLogs, func(t corev1.TimestampedLog) bool {
			return !t.GetTimestamp().AsTime().After(lastLogTimestamp)
		})
		for i, log := range allLogs {
			printStatusLog(log)
			if i == len(allLogs)-1 {
				lastLogTimestamp = log.GetTimestamp().AsTime()
			}
		}

		if status.State == corev1.TaskState_Completed ||
			status.State == corev1.TaskState_Failed ||
			status.State == corev1.TaskState_Canceled {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func printStatusLog(log corev1.TimestampedLog) {
	timestamp := log.GetTimestamp().AsTime().Format(time.StampMilli)
	msg := log.GetMsg()
	if strings.HasPrefix(msg, "internal:") {
		msg = chalk.Inverse.TextStyle(msg)
	}
	msg = fmt.Sprintf("[%s] %s", timestamp, msg)
	switch log.GetLogLevel() {
	case slog.LevelDebug:
		statusLog.Debug(msg)
	case slog.LevelInfo:
		statusLog.Info(msg)
	case slog.LevelWarn:
		statusLog.Warn(msg)
	case slog.LevelError:
		statusLog.Error(msg)
	default:
		statusLog.Info(msg)
	}
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildCapabilityCmd())
}
