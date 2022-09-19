package commands

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/pkg/realtime"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func BuildRealtimeCmd() *cobra.Command {
	lg := logger.New()
	var configLocation string

	cmd := &cobra.Command{
		Use:   "realtime",
		Short: "Run the Opni real-time server",
		Run: func(cmd *cobra.Command, args []string) {
			objects := cliutil.LoadConfigObjectsOrDie(configLocation, lg)
			objects.Visit(
				func(config *v1beta1.RealtimeServerConfig) {
					ctx, cancel := context.WithCancel(waitctx.Background())
					defer cancel()
					server, err := realtime.NewServer(&config.Spec)
					if err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to create realtime server")
					}
					if err := server.Start(ctx); err != nil {
						lg.With(
							zap.Error(err),
						).Fatal("failed to start realtime server")
					}
					waitctx.Wait(ctx)
				},
			)

		},
	}
	cmd.Flags().StringVar(&configLocation, "config", "", "Absolute path to a config file")
	return cmd
}
