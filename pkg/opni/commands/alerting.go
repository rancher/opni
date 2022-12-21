//go:build !noalertmanager

package commands

import (
	"flag"
	"path"

	"github.com/opentracing/opentracing-go/log"
	alertmanager_internal "github.com/rancher/opni/internal/alerting/alertmanager"
	"github.com/rancher/opni/internal/alerting/syncer"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/spf13/cobra"
)

func BuildAlertingComponents() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerting-server",
		Short: "Run one of the alerting components",
	}
	cmd.AddCommand(BuildAlertingSyncer())
	cmd.AddCommand(BuildAlertManager())

	return cmd
}

func BuildAlertingSyncer() *cobra.Command {
	var gatewayAddress string
	var configFilePath string
	var listenAddress string
	var alertmanagerAddress string
	var pprofPort int64
	var profileBlockRate int64
	cmd := &cobra.Command{
		Use:   "syncer",
		Short: "Run the side-car Alertmanager alerting syncer server",
		Run: func(cmd *cobra.Command, args []string) {
			tracing.Configure("alerting-syncer")
			flag.CommandLine = flag.NewFlagSet("syncer", flag.ExitOnError)

			serverConfig := &alertingv1.SyncerConfig{
				GatewayJoinAddress:     gatewayAddress,
				AlertmanagerConfigPath: configFilePath,
				ListenAddress:          listenAddress,
				AlertmanagerAddress:    alertmanagerAddress,
				HookListenAddress:      path.Join(listenAddress, shared.AlertingDefaultHookName),
				PprofPort:              pprofPort,
				ProfileBlockRate:       profileBlockRate,
			}

			if err := serverConfig.Validate(); err != nil {
				lg.Fatal(err)
			}

			err := syncer.Main(cmd.Context(), serverConfig)
			if err != nil {
				log.Error(err)
			}
		},
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&configFilePath, "config.file", shared.ConfigMountPath, "the alertmanager config file to sync to")
	cmd.Flags().StringVar(&listenAddress, "listen.address", ":8080", "the address to listen on")
	cmd.Flags().StringVar(&alertmanagerAddress, "alertmanager.address", "http://localhost:9093", "the address of the remote alertmanager instance to sync")
	cmd.Flags().Int64Var(&pprofPort, "pprof.port", 0, "the port to listen on for pprof")
	cmd.Flags().Int64Var(&profileBlockRate, "profile.block-rate", 0, "the rate at which to profile blocking operations")
	cmd.Flags().StringVar(&gatewayAddress, "gateway.join.address", "", "the address of the gateway to join")
	return cmd
}

func BuildAlertManager() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "alertmanager",
		Short:              "Run the embedded Alertmanager server",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			tracing.Configure("alertmanager")
			flag.CommandLine = flag.NewFlagSet("alertmanager", flag.ExitOnError)
			alertmanager_internal.Main(append([]string{"alertmanager"}, args...))
		},
	}
	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildAlertingComponents())
}
