/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"github.com/kralicky/highlander"
	upgraderesponder "github.com/longhorn/upgrade-responder/client"
	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	opensearchcontrollers "opensearch.opster.io/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/manager"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	Version  = "dev"
)

const (
	upgradeResponderAddress = "https://opni-usage.danbason.dev/v1/checkupgrade"
)

func init() {
	apis.InitScheme(scheme)
}

func main() {
	if err := run(); err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}
}

func run() error {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		disableUsage         bool
		echoVersion          bool
	)

	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.BoolVar(&disableUsage, "disable-usage", false, "Disable anonymous Opni usage tracking.")
	pflag.BoolVarP(&echoVersion, "version", "v", false, "print the version and exit")
	features.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)
	opts := zap.Options{
		Development: true,
	}

	pflag.Parse()

	if echoVersion {
		fmt.Println(Version)
		return nil
	}

	ctrl.SetLogger(zap.New(
		zap.UseFlagOptions(&opts),
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

	if err = (&v1beta1.LogAdapter{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "LogAdapter")
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
}
