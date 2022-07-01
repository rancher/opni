package commands

import (
	"fmt"
	"os"

	"github.com/kralicky/highlander"
	upgraderesponder "github.com/longhorn/upgrade-responder/client"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/manager"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	opensearchcontrollers "opensearch.opster.io/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	Version  = "dev"
)

const (
	upgradeResponderAddress = "https://upgrades.opni-upgrade-responder.livestock.rancher.io/v1/checkupgrade"
)

func init() {
	apis.InitScheme(scheme)
}

func BuildManagerCmd() *cobra.Command {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		disableUsage         bool
		echoVersion          bool
		logLevel             string
	)
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Run the Opni Manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			tracing.Configure("manager")

			if echoVersion {
				fmt.Println(Version)
				return nil
			}

			if os.Getenv("DO_NOT_TRACK") == "1" {
				disableUsage = true
			}

			ctrl.SetLogger(zap.New(
				zap.Level(util.Must(zapcore.ParseLevel(logLevel))),
				zap.Encoder(zapcore.NewConsoleEncoder(util.EncoderConfig)),
			))

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

			if !disableUsage && Version != "dev" {
				upgradeRequester := manager.UpgradeRequester{Version: Version}
				upgradeRequester.SetupLoggerWithManager(mgr)
				setupLog.Info("Usage tracking enabled", "current-version", Version)
				upgradeChecker := upgraderesponder.NewUpgradeChecker(upgradeResponderAddress, &upgradeRequester)
				upgradeChecker.Start()
				defer upgradeChecker.Stop()
			}

			if err = (&controllers.OpniClusterReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "OpniCluster")
				return err
			}

			if err = (&controllers.LoggingReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "Logging")
				return err
			}

			if err = (&controllers.LogAdapterReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "LogAdapter")
				return err
			}

			if err = (&controllers.PretrainedModelReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "PretrainedModel")
				return err
			}

			if err = (&controllers.LoggingClusterReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "LoggingCluster")
				return err
			}

			if err = (&controllers.MulticlusterRoleBindingReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "MulticlusterRoleBinding")
				return err
			}

			if err = (&controllers.LoggingClusterBindingReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "LoggingClusterBinding")
				return err
			}

			if err = (&controllers.MulticlusterUserReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "MulticlusterUser")
				return err
			}

			if err = (&controllers.DataPrepperReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "DataPrepper")
				return err
			}

			if err = (&controllers.PretrainedModelReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "PretrainedModel")
				return err
			}

			if err = (&controllers.GatewayReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "Gateway")
				return err
			}

			if err = (&controllers.MonitoringReconciler{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "MonitoringCluster")
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

			if err = (&opensearchcontrollers.OpenSearchClusterReconciler{
				Client:   mgr.GetClient(),
				Scheme:   mgr.GetScheme(),
				Recorder: mgr.GetEventRecorderFor("containerset-controller"),
			}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "OpenSearchCluster")
				return err
			}

			if features.DefaultMutableFeatureGate.Enabled(features.NodeFeatureDiscoveryOperator) {
				if err = (&controllers.NodeFeatureDiscoveryReconciler{}).SetupWithManager(mgr); err != nil {
					setupLog.Error(err, "unable to create controller", "controller", "NodeFeatureDiscovery")
					return err
				}
			}

			if features.DefaultMutableFeatureGate.Enabled(features.GPUOperator) {
				if err = (&controllers.ClusterPolicyReconciler{}).SetupWithManager(mgr); err != nil {
					setupLog.Error(err, "unable to create controller", "controller", "ClusterPolicy")
					return err
				}
				if err = (&controllers.GpuPolicyAdapterReconciler{}).SetupWithManager(mgr); err != nil {
					setupLog.Error(err, "unable to create controller", "controller", "GpuPolicyAdapter")
					return err
				}
			}

			if err = (&v1beta2.OpniCluster{}).SetupWebhookWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", "OpniCluster")
				return err
			}
			if err = (&v1beta2.LogAdapter{}).SetupWebhookWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", "LogAdapter")
				return err
			}
			if err = (&v1beta2.GpuPolicyAdapter{}).SetupWebhookWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", "GpuPolicyAdapter")
				return err
			}
			if err = (&v1beta2.PretrainedModel{}).SetupWebhookWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", "PretrainedModel")
				return err
			}

			if err := highlander.NewFor(&v1beta1.OpniCluster{}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create webhook", "webhook", "OpniCluster")
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

			setupLog.Info("starting manager")
			if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warning, error)")
	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().BoolVar(&disableUsage, "disable-usage", false, "Disable anonymous Opni usage tracking.")
	cmd.Flags().BoolVarP(&echoVersion, "version", "v", false, "print the version and exit")
	features.DefaultMutableFeatureGate.AddFlag(cmd.Flags())

	return cmd
}
