package commands

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	agentv2 "github.com/rancher/opni/pkg/agent/v2"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BuildAgentV2Cmd() *cobra.Command {
	var configFile, logLevel string
	cmd := &cobra.Command{
		Use:   "agentv2",
		Short: "Run the v2 agent",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := waitctx.FromContext(cmd.Context())

			// tracing.Configure("agentv2")
			agentlg = logger.New(logger.WithLogLevel(util.Must(zapcore.ParseLevel(logLevel))))

			if configFile == "" {
				// find config file
				path, err := config.FindConfig()
				if err != nil {
					if errors.Is(err, config.ErrConfigNotFound) {
						wd, _ := os.Getwd()
						agentlg.Fatalf(`could not find a config file in ["%s","/etc/opni-monitoring"], and --config was not given`, wd)
					}
					agentlg.With(
						zap.Error(err),
					).Fatal("an error occurred while searching for a config file")
				}
				agentlg.With(
					"path", path,
				).Info("using config file")
				configFile = path
			}

			objects, err := config.LoadObjectsFromFile(configFile)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to load config")
			}
			var agentConfig *v1beta1.AgentConfig
			if ok := objects.Visit(func(config *v1beta1.AgentConfig) {
				agentConfig = config
			}); !ok {
				agentlg.Fatal("no agent config found in config file")
			}

			if agentConfig.Spec.Profiling {
				fmt.Fprintln(os.Stderr, chalk.Yellow.Color("Profiling is enabled. This should only be used for debugging purposes."))
				runtime.SetBlockProfileRate(10000)
				runtime.SetMutexProfileFraction(100)
			}

			bootstrapper, err := configureBootstrap(agentConfig)
			if err != nil {
				agentlg.With(
					zap.Error(err),
				).Fatal("failed to configure bootstrap")
			}

			pl := plugins.NewPluginLoader()

			p, err := agentv2.New(ctx, agentConfig, pl,
				agentv2.WithBootstrapper(bootstrapper),
			)
			if err != nil {
				agentlg.Error(err)
				return
			}

			pl.Hook(hooks.OnLoadingCompleted(func(numLoaded int) {
				lg.Infof("loaded %d plugins", numLoaded)
			}))

			pl.Hook(hooks.OnLoadingCompleted(func(int) {
				waitctx.AddOne(ctx)
				defer waitctx.Done(ctx)
				if err := p.ListenAndServe(ctx); err != nil {
					lg.With(
						zap.Error(err),
					).Warn("agent server exited with error")
				}
			}))

			pl.LoadPlugins(ctx, agentConfig.Spec.Plugins, plugins.AgentScheme)

			<-ctx.Done()
			waitctx.Wait(ctx)
		},
	}
	cmd.Flags().StringVar(&configFile, "config", "", "Absolute path to a config file")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warning, error)")
	return cmd
}
