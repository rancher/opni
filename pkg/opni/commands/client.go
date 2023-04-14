//go:build !nomanager

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
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/manager"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/pkg/versions"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
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
		opniCentral  bool
	)

	run := func(signalCtx context.Context) error {
		tracing.Configure("client")

		if echoVersion {
			fmt.Println(versions.Version)
			return nil
		}

		if os.Getenv("DO_NOT_TRACK") == "1" {
			disableUsage = true
		}

		level, err := zapcore.ParseLevel(logLevel)
		if err != nil {
			return err
		}

		ctrl.SetLogger(k8sutil.NewControllerRuntimeLogger(level))

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
				Version:     versions.Version,
				InstallType: manager.InstallTypeAgent,
			}
			upgradeRequester.SetupLoggerWithManager(mgr)
			setupLog.Info("Usage tracking enabled", "current-version", versions.Version)
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

		if err = (&controllers.LoggingDataPrepperReconciler{
			OpniCentral: opniCentral,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging DataPrepper")
			return err
		}

		if err = (&controllers.CoreCollectorReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Core Collector")
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
	cmd.Flags().BoolVarP(&opniCentral, "central", "c", false, "run controllers in Opni central cluster mode")
	cmd.Flags().BoolVarP(&echoVersion, "version", "v", false, "print the version and exit")
	features.DefaultMutableFeatureGate.AddFlag(cmd.Flags())

	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildClientCmd())
}
