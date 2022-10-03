package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	upgraderesponder "github.com/longhorn/upgrade-responder/client"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/manager"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	apis.InitScheme(scheme)
}

func BuildClientCmd() *cobra.Command {
	var (
		metricsAddr  string
		probeAddr    string
		disableUsage bool
		echoVersion  bool
		logLevel     string
	)

	run := func(signalCtx context.Context) error {
		tracing.Configure("client")

		if echoVersion {
			fmt.Println(util.Version)
			return nil
		}

		if os.Getenv("DO_NOT_TRACK") == "1" {
			disableUsage = true
		}

		ctrl.SetLogger(zap.New(
			zap.Level(util.Must(zapcore.ParseLevel(logLevel))),
			zap.Encoder(zapcore.NewConsoleEncoder(testutil.EncoderConfig)),
		))

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   9443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         false,
		})
		if err != nil {
			setupLog.Error(err, "unable to start client manager")
			return err
		}

		var upgradeChecker *upgraderesponder.UpgradeChecker
		if !(disableUsage || common.DisableUsage) {
			upgradeRequester := manager.UpgradeRequester{
				Version:     util.Version,
				InstallType: manager.InstallTypeAgent,
			}
			upgradeRequester.SetupLoggerWithManager(mgr)
			setupLog.Info("Usage tracking enabled", "current-version", util.Version)
			upgradeChecker = upgraderesponder.NewUpgradeChecker(upgradeResponderAddress, &upgradeRequester)
			upgradeChecker.Start()
			defer upgradeChecker.Stop()
		}

		if err = (&controllers.LoggingReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging")
			return err
		}

		if err = (&controllers.LoggingLogAdapterReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging LogAdapter")
			return err
		}

		if err = (&controllers.LoggingDataPrepperReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging DataPrepper")
			return err
		}

		// +kubebuilder:scaffold:builder

		if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up health check")
			return err
		}
		if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			return err
		}

		ctx, cancel := context.WithCancel(waitctx.FromContext(signalCtx))

		errC := make(chan struct{})
		waitctx.Go(ctx, func() {
			setupLog.Info("starting manager")
			if err := mgr.Start(ctx); err != nil {
				setupLog.Error(err, "error running manager")
				close(errC)
			}
		})

		reloadC := make(chan struct{})

		select {
		case <-reloadC:
			setupLog.Info("reload signal received")
			cancel()
			// wait for graceful shutdown
			waitctx.Wait(ctx, 5*time.Second)
			return nil
		case <-errC:
			setupLog.Error(nil, "error running manager")
			cancel()
			return errors.New("error running manager")
		}
	}

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Run the Opni Client Manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			signalCtx := ctrl.SetupSignalHandler()
			for {
				if err := run(signalCtx); err != nil {
					return err
				}
			}
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warning, error)")
	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":7080", "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":7081", "The address the probe endpoint binds to.")
	cmd.Flags().BoolVarP(&echoVersion, "version", "v", false, "print the version and exit")
	features.DefaultMutableFeatureGate.AddFlag(cmd.Flags())

	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildClientCmd())
}
