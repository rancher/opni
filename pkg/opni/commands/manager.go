//go:build !minimal && !cli

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	upgraderesponder "github.com/longhorn/upgrade-responder/client"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/manager"
	"github.com/rancher/opni/pkg/versions"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	opensearchcontrollers "opensearch.opster.io/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

func BuildManagerCmd() *cobra.Command {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		disableUsage         bool
		echoVersion          bool
		logLevel             string
	)

	run := func(signalCtx context.Context) error {
		tracing.Configure("manager")

		if echoVersion {
			fmt.Println(versions.Version)
			return nil
		}

		if os.Getenv("DO_NOT_TRACK") == "1" {
			disableUsage = true
		}

		level := logger.ParseLevel(logLevel)

		ctrl.SetLogger(k8sutil.NewControllerRuntimeLogger(level))

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   9443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "98e737d4.opni.io",
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			return err
		}

		var upgradeChecker *upgraderesponder.UpgradeChecker
		if !(disableUsage || common.DisableUsage) {
			upgradeRequester := manager.UpgradeRequester{
				Version:     versions.Version,
				InstallType: manager.InstallTypeManager,
			}
			upgradeRequester.SetupLoggerWithManager(mgr)
			setupLog.Info("Usage tracking enabled", "current-version", versions.Version)
			upgradeChecker = upgraderesponder.NewUpgradeChecker(upgradeResponderAddress, &upgradeRequester)
			upgradeChecker.Start()
			defer upgradeChecker.Stop()
		}

		if err = (&controllers.AIOpniClusterReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "AI OpniCluster")
			return err
		}

		if err = (&controllers.AIPretrainedModelReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "AI PretrainedModel")
			return err
		}

		if err = (&controllers.CoreLoggingClusterReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Core LoggingCluster")
			return err
		}

		if err = (&controllers.LoggingMulticlusterUserReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging MulticlusterUser")
			return err
		}

		if err = (&controllers.LoggingMulticlusterRoleBindingReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging MulticlusterRoleBinding")
			return err
		}

		if err = (&controllers.LoggingClusterBindingReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "LoggingClusterBinding")
			return err
		}

		if err = (&controllers.LoggingOpensearchRepositoryReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging OpensearchRepository")
			return err
		}

		if err = (&controllers.LoggingSnapshotReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging Snapshot")
			return err
		}

		if err = (&controllers.LoggingRecurringSnapshotReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Logging RecurringSnapshot")
			return err
		}

		if err = (&controllers.CoreGatewayReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Core Gateway")
			return err
		}

		if err = (&controllers.CoreMonitoringReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Core MonitoringCluster")
			return err
		}

		if err = (&controllers.NatsClusterReonciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "NatsCluster")
			return err
		}
		if err = (&controllers.LoggingPreprocessorReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Preprocessor")
			return err
		}

		if err = (&controllers.GrafanaReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Grafana")
			return err
		}

		if err = (&controllers.GrafanaDashboardReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "GrafanaDashboard")
			return err
		}

		if err = (&controllers.GrafanaDatasourceReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "GrafanaDatasource")
			return err
		}

		if err = (&controllers.CoreAlertingReconciler{}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Core AlertingCluster")
			return err
		}

		if err = (&opensearchcontrollers.OpenSearchClusterReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("containerset-controller"),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "OpenSearchCluster")
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

		ctx, cancel := context.WithCancel(signalCtx)

		features.PopulateFeatures(ctx, ctrl.GetConfigOrDie())
		features.FeatureList.WatchConfigMap()

		if err = (&controllers.LoggingOpniOpensearchReconciler{}).SetupWithManager(mgr); err != nil {
			defer cancel()
			setupLog.Error(err, "unable to create controller", "controller", "Logging OpniOpensearch")
			return err
		}

		errC := make(chan struct{})
		go func() {
			setupLog.Info("starting manager")
			if err := mgr.Start(ctx); err != nil {
				setupLog.Error(err, "error running manager")
				close(errC)
			}
		}()

		reloadC := make(chan struct{})
		go func() {
			fNotify := features.FeatureList.NotifyChange()
			<-fNotify
			close(reloadC)
		}()

		select {
		case <-reloadC:
			setupLog.Info("reload signal received")
			cancel()
			<-errC
			return nil
		case <-errC:
			setupLog.Error(nil, "error running manager")
			cancel()
			return errors.New("error running manager")
		}
	}

	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Run the Opni Manager",
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
	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().BoolVarP(&echoVersion, "version", "v", false, "print the version and exit")
	features.DefaultMutableFeatureGate.AddFlag(cmd.Flags())

	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildManagerCmd())
}
