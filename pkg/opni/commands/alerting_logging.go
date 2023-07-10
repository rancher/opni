package commands

// import (
// 	// "github.com/rancher/opni/plugins/logging/pkg/apis/alerting"
// 	"bytes"
// 	"encoding/json"
// 	"fmt"

// 	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
// 	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
// 	"github.com/spf13/cobra"
// 	"google.golang.org/protobuf/types/known/emptypb"
// )

// func BuildLoggingAlertingRootCmd() *cobra.Command {
// 	cmd := &cobra.Command{
// 		Use:   "alerting",
// 		Short: "Interact with alerting plugin APIs",
// 	}
// 	cmd.AddCommand(BuildGetMonitorCmd())
// 	cmd.AddCommand(BuildGetNotificationChannelCmd())
// 	return cmd
// }

// func BuildGetMonitorCmd() *cobra.Command {
// 	var (
// 		monitorId string
// 	)
// 	cmd := &cobra.Command{
// 		Use:   "monitor",
// 		Short: "get a monitor with id",
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			monitor, err := loggingMonitorClient.GetMonitor(cmd.Context(), &corev1.Reference{Id: monitorId})
// 			if err != nil {
// 				return err
// 			}
// 			res := types.MonitorSpec{}
// 			err = json.NewDecoder(bytes.NewReader(monitor.Spec)).Decode(&res)
// 			if err != nil {
// 				return err
// 			}
// 			fmt.Print(res)
// 			return nil
// 		},
// 	}
// 	cmd.Flags().StringVar(&monitorId, "id", "", "monitor id")
// 	return cmd
// }

// func BuildGetNotificationChannelCmd() *cobra.Command {
// 	cmd := &cobra.Command{
// 		Use:   "notification",
// 		Short: "get notification channels",
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			channels, err := loggingNotificationClient.ListNotifications(cmd.Context(), &emptypb.Empty{})
// 			if err != nil {
// 				return err
// 			}
// 			res := types.ListChannelResponse{}
// 			if err := json.NewDecoder(bytes.NewReader(channels.GetList())).Decode(&res); err != nil {
// 				return err
// 			}
// 			return nil
// 		},
// 	}
// 	return cmd
// }
