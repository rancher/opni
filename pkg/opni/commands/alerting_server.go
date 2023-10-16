//go:build !minimal && !cli

package commands

import (
	"flag"
	"fmt"
	"path"
	"strings"

	alertmanager_internal "github.com/rancher/opni/internal/alerting/alertmanager"
	"github.com/rancher/opni/internal/alerting/syncer"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/spf13/cobra"
)

const syncerPrefix = "syncer"

func BuildAlertingComponents() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerting-server",
		Short: "Run one of the alerting components",
	}
	cmd.AddCommand(BuildAlertingSyncer())
	cmd.AddCommand(BuildAlertManager())

	return cmd
}

func addSyncerPrefix(input string) string {
	return fmt.Sprintf("%s.%s", syncerPrefix, input)
}

func hasSyncerPrefix(input string) bool {
	return strings.HasPrefix(input, fmt.Sprintf("--%s", syncerPrefix))
}

func BuildAlertingSyncer() *cobra.Command {
	var syncerGatewayJoinAddress string
	var syncerAlertManagerConfigFilePath string
	var syncerListenAddress string
	var syncerAlertManagerAddress string
	var syncerPprofPort int64
	var syncerProfileBlockRate int64

	var tlsServerCa string
	var tlsClientCa string
	var tlsClientCert string
	var tlsClientKey string

	cmd := &cobra.Command{
		Use:                "syncer",
		Short:              "Run the side-car Alertmanager alerting syncer server",
		Long:               "Note: this command is only intended to be run as a side-car container to the Alertmanager server.",
		DisableFlagParsing: false,
		Run: func(cmd *cobra.Command, args []string) {
			tracing.Configure("alerting-syncer")
			flag.CommandLine = flag.NewFlagSet("syncer", flag.ExitOnError)

			serverConfig := &alertingv1.SyncerConfig{
				GatewayJoinAddress:     syncerGatewayJoinAddress,
				AlertmanagerConfigPath: syncerAlertManagerConfigFilePath,
				ListenAddress:          syncerListenAddress,
				AlertmanagerAddress:    syncerAlertManagerAddress,
				HookListenAddress:      path.Join(syncerListenAddress, shared.AlertingDefaultHookName),
				PprofPort:              syncerPprofPort,
				ProfileBlockRate:       syncerProfileBlockRate,
			}

			if err := serverConfig.Validate(); err != nil {
				lg.Error("error", logger.Err(err))
				os.Exit(1)
			}
			lg.Debug("syncer gateway join address" + syncerGatewayJoinAddress)

			lg.With(
				"server-ca", tlsServerCa,
				"client-ca", tlsClientCa,
				"client-cert", tlsClientCert,
				"client-key", tlsClientKey,
			).Info("loading tls config...")
			clientConfig := &client.TLSClientConfig{
				ServerCA:   tlsServerCa,
				ClientCA:   tlsClientCa,
				ClientCert: tlsClientCert,
				ClientKey:  tlsClientKey,
			}
			tlsConfig, err := clientConfig.Init()
			if err != nil {
				lg.Errorf("failed to load tls client config %s, exiting...", err)
			}
			err = syncer.Main(cmd.Context(), serverConfig, tlsConfig)
			if err != nil {
				lg.Error("error", logger.Err(err))
			}
		},
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&syncerAlertManagerConfigFilePath, addSyncerPrefix("alertmanager.config.file"), shared.ConfigMountPath, "the alertmanager config file to sync to")
	cmd.Flags().StringVar(&syncerListenAddress, addSyncerPrefix("listen.address"), ":8080", "the address to listen on")
	cmd.Flags().StringVar(&syncerAlertManagerAddress, addSyncerPrefix("alertmanager.address"), "127.0.0.1:9093", "the address of the remote alertmanager instance to sync")
	cmd.Flags().Int64Var(&syncerPprofPort, addSyncerPrefix("pprof.port"), 0, "the port to listen on for pprof")
	cmd.Flags().Int64Var(&syncerProfileBlockRate, addSyncerPrefix("profile.block-rate"), 0, "the rate at which to profile blocking operations")
	cmd.Flags().StringVar(&syncerGatewayJoinAddress, addSyncerPrefix("gateway.join.address"), "", "the address of the gateway to join")
	cmd.Flags().StringVar(&tlsServerCa, addSyncerPrefix("tls.server.ca"), "/run/certs/server/ca.crt", "the path to the server ca")
	cmd.Flags().StringVar(&tlsClientCa, addSyncerPrefix("tls.client.ca"), "/run/certs/client/ca.crt", "the path to the client ca")
	cmd.Flags().StringVar(&tlsClientCert, addSyncerPrefix("tls.client.cert"), "/run/certs/client/tls.crt", "the path to the client cert")
	cmd.Flags().StringVar(&tlsClientKey, addSyncerPrefix("tls.client.key"), "/run/certs/client/tls.key", "the path to the client key")
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
