package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	dpb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/jhump/protoreflect/desc"
	"github.com/mitchellh/mapstructure"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap/zapcore"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

var statusLog = logger.New(
	logger.WithDisableCaller(),
	logger.WithTimeEncoder(func(time.Time, zapcore.PrimitiveArrayEncoder) {}),
)

func BuildCapabilityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "capability",
		Aliases: []string{"cap"},
		Short:   "Manage cluster capabilities",
	}
	cmd.AddCommand(BuildCapabilityListCmd())
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
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderCapabilityList(list))
		},
	}
	return cmd
}

func BuildCapabilityUninstallCmd() *cobra.Command {
	var options []string
	var follow bool
	var optionsHelp []string
	desc, err := desc.LoadMessageDescriptorForMessage(&capabilityv1.UninstallOptions{})
	if err == nil {
		for _, field := range desc.GetFields() {
			var typeName string
			if field.GetType() == dpb.FieldDescriptorProto_TYPE_MESSAGE {
				typeName = strings.ToLower(field.GetMessageType().GetName())
			} else {
				typeName = strings.TrimPrefix(strings.ToLower(field.GetType().String()), "type_")
			}
			optionsHelp = append(optionsHelp, fmt.Sprintf("%s: %s", field.GetName(), typeName))
		}
	}
	cmd := &cobra.Command{
		Use:   "uninstall [--option key=value ...] <cluster-id> <capability-name>",
		Short: "Uninstall a capability from a cluster",
		Long:  fmt.Sprintf("Available options:\n%s", strings.Join(optionsHelp, "\n")),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			optionMap := map[string]string{}
			for _, option := range options {
				k, v, ok := strings.Cut(option, "=")
				if !ok {
					return fmt.Errorf("invalid option: %s", option)
				}
				optionMap[k] = v
			}
			options := capabilityv1.UninstallOptions{}
			for k, v := range optionMap {
				field := desc.FindFieldByJSONName(k)
				if field == nil {
					return fmt.Errorf("unknown option: %s", k)
				}
				if field.GetType() == dpb.FieldDescriptorProto_TYPE_MESSAGE {
					// invoke custom unmarshaling logic (for time/duration wrapper types)
					if err := protojson.Unmarshal([]byte(fmt.Sprintf(`{%q: %q}`, k, v)), &options); err != nil {
						fmt.Println(err)
						return fmt.Errorf("invalid value for option %s: %s", k, v)
					}
				} else {
					// weakly decode strings to primitive types
					if err := mapstructure.WeakDecode(map[string]any{k: v}, &options); err != nil {
						fmt.Println(err)
						return fmt.Errorf("invalid value for option %s: %s", k, v)
					}
				}
			}
			_, err := mgmtClient.UninstallCapability(cmd.Context(), &managementv1.CapabilityUninstallRequest{
				Name: args[1],
				Target: &capabilityv1.UninstallRequest{
					Cluster: &corev1.Reference{
						Id: args[0],
					},
					Options: &options,
				},
			})
			if err != nil {
				return fmt.Errorf("uninstall failed: %w", err)
			}

			lg.Info("Uninstall request submitted successfully")
			if !follow {
				return nil
			}

			lg.Info("Watching for progress updates...")
			logTaskProgress(cmd.Context(), args[0], args[1])
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&options, "option", "o", nil, "option key=value")
	cmd.Flags().BoolVar(&follow, "follow", true, "follow progress of uninstall task")
	return cmd
}

func BuildCapabilityCancelUninstallCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "cancel-uninstall <cluster-id> <capability-name>",
		Short: "Cancel an in-progress uninstall of a capability",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			_, err := mgmtClient.CancelCapabilityUninstall(cmd.Context(), &managementv1.CapabilityUninstallCancelRequest{
				Name: args[1],
				Cluster: &corev1.Reference{
					Id: args[0],
				},
			})
			if err != nil {
				lg.Fatal(err)
			}
			lg.Info("Cancel request submitted successfully")
			if !follow {
				return
			}
			lg.Info("Watching for progress updates...")
			logTaskProgress(cmd.Context(), args[0], args[1])
		},
	}
	cmd.Flags().BoolVar(&follow, "follow", true, "follow progress of uninstall task")
	return cmd
}

func BuildCapabilityStatusCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "status <cluster-id> <capability-name>",
		Short: "Show the status of a capability, or the status of an in-progress uninstall operation",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cluster, err := mgmtClient.GetCluster(cmd.Context(), &corev1.Reference{
				Id: args[0],
			})
			if err != nil {
				lg.Fatal(err)
			}
			for _, cap := range cluster.GetCapabilities() {
				if cap.Name != args[1] {
					continue
				}
				if cap.DeletionTimestamp != nil {
					fmt.Println(chalk.Yellow.Color("Uninstalling"))
					if follow {
						logTaskProgress(cmd.Context(), args[0], args[1])
					}
					return
				}
				fmt.Println(chalk.Green.Color("Installed"))
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
		slices.SortFunc(allLogs, func(a, b corev1.TimestampedLog) bool {
			return a.GetTimestamp().AsTime().Before(b.GetTimestamp().AsTime())
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
	case zapcore.DebugLevel:
		statusLog.Debug(msg)
	case zapcore.InfoLevel:
		statusLog.Info(msg)
	case zapcore.WarnLevel:
		statusLog.Warn(msg)
	case zapcore.ErrorLevel:
		statusLog.Error(msg)
	default:
		statusLog.Info(msg)
	}
}
