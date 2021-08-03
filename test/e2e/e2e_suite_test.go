package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/k3d/v4/pkg/client"
	"github.com/rancher/k3d/v4/pkg/config"
	"github.com/rancher/k3d/v4/pkg/config/v1alpha2"
	"github.com/rancher/k3d/v4/pkg/runtimes"
	"github.com/rancher/k3d/v4/pkg/types"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/controllers/demo"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	if testing.Short() {
		t.Skip("Skipping e2e tests")
	}
	RunSpecs(t, "E2E Tests")
}

var (
	clusterName = "opni-k3d-e2e-test-cluster"
	testEnv     *envtest.Environment
	stopEnv     context.CancelFunc
	k8sClient   crclient.Client
	restConfig  *rest.Config
	useExisting bool
)

func deleteTestClusterIfExists() {
	cluster, err := client.ClusterGet(context.Background(), runtimes.Docker, &types.Cluster{
		Name: clusterName,
	})
	if err == nil {
		if err := client.ClusterDelete(context.Background(), runtimes.Docker, cluster, types.ClusterDeleteOpts{}); err != nil {
			logf.Log.Error(err, "Failed to delete existing cluster")
		}
	}
}

var _ = BeforeSuite(func() {
	logf.SetLogger(util.NewTestLogger())

	if str, ok := os.LookupEnv("E2E_USE_EXISTING"); ok {
		if value, err := strconv.ParseBool(str); err == nil {
			logf.Log.Info("Using existing cluster")
			useExisting = value
		}
	}

	if !useExisting {
		deleteTestClusterIfExists()
		freePort, err := freeport.GetFreePort()
		Expect(err).NotTo(HaveOccurred())
		simpleConfig := v1alpha2.SimpleConfig{
			Name:    clusterName,
			Image:   "rancher/k3s:latest",
			Servers: 1,
			Agents:  1,
			ExposeAPI: v1alpha2.SimpleExposureOpts{
				HostIP:   "127.0.0.1",
				HostPort: fmt.Sprint(freePort),
			},
			Ports: []v1alpha2.PortWithNodeFilters{
				{
					Port:        "8081:80",
					NodeFilters: []string{"loadbalancer"},
				},
			},
			Options: v1alpha2.SimpleConfigOptions{
				K3sOptions: v1alpha2.SimpleConfigOptionsK3s{
					ExtraServerArgs: []string{
						"--log=/var/log/k3s.log",
						"--alsologtostderr",
					},
				},
			},
		}

		ctx := context.Background()
		conf, err := config.TransformSimpleToClusterConfig(
			ctx, runtimes.Docker, simpleConfig)
		Expect(err).NotTo(HaveOccurred())

		err = client.ClusterRun(ctx, runtimes.Docker, conf)
		Expect(err).NotTo(HaveOccurred())

		kubeconfig, err := client.KubeconfigGet(ctx, runtimes.Docker, &conf.Cluster)
		Expect(err).NotTo(HaveOccurred())

		restConfig, err = clientcmd.NewDefaultClientConfig(*kubeconfig, nil).ClientConfig()
		Expect(err).NotTo(HaveOccurred())
	} else {
		fmt.Println("KUBECONFIG=" + os.Getenv("KUBECONFIG"))
	}

	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../config/crd/bases",
			"../../config/crd/logging",
			"../resources",
		},
		BinaryAssetsDirectory: "../../testbin/bin",
		UseExistingCluster:    pointer.Bool(true), // This should always be true
		Config:                restConfig,         // this will be nil if E2E_USE_EXISTING is set
		Scheme:                scheme.Scheme,
		CRDInstallOptions: envtest.CRDInstallOptions{
			CleanUpAfterUse: useExisting,
		},
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1",
						Port:    fmt.Sprint(port),
					},
				},
			},
		},
	}

	var mgr manager.Manager
	stopEnv, mgr, k8sClient = test.RunTestEnvironment(testEnv,
		&demo.OpniDemoReconciler{},
		&controllers.LoggingReconciler{},
		&controllers.LogAdapterReconciler{},
		&controllers.PretrainedModelReconciler{},
		&controllers.OpniClusterReconciler{},
	)
	if restConfig == nil {
		restConfig = mgr.GetConfig()
	}
})

var _ = AfterSuite(func() {
	stopEnv()
	if !useExisting {
		deleteTestClusterIfExists()
	}
})
