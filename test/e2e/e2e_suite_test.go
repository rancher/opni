package e2e

import (
	"context"
	"fmt"
	"testing"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/k3d/v4/pkg/client"
	"github.com/rancher/k3d/v4/pkg/config"
	"github.com/rancher/k3d/v4/pkg/config/v1alpha2"
	"github.com/rancher/k3d/v4/pkg/runtimes"
	"github.com/rancher/k3d/v4/pkg/types"
	demov1alpha1 "github.com/rancher/opni/api/v1alpha1"
	"github.com/rancher/opni/api/v1beta1"
	"github.com/rancher/opni/controllers"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Tests")
}

var clusterName = "opni-k3d-e2e-test-cluster"
var testEnv *envtest.Environment
var k8sClient crclient.Client

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
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

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
	}

	ctx := context.Background()
	conf, err := config.TransformSimpleToClusterConfig(
		ctx, runtimes.Docker, simpleConfig)
	Expect(err).NotTo(HaveOccurred())

	err = client.ClusterRun(ctx, runtimes.Docker, conf)
	Expect(err).NotTo(HaveOccurred())

	kubeconfig, err := client.KubeconfigGet(ctx, runtimes.Docker, &conf.Cluster)
	Expect(err).NotTo(HaveOccurred())

	restConfig, err := clientcmd.NewDefaultClientConfig(*kubeconfig, nil).ClientConfig()
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../config/crd/bases",
			"../../controllers/test",
		},
		BinaryAssetsDirectory: "../../testbin/bin",
		UseExistingCluster:    pointer.Bool(true),
		Config:                restConfig,
		Scheme:                scheme.Scheme,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = demov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = helmv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiextv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	err = (&controllers.OpniClusterReconciler{
		Client: k8sClient,
		Log:    ctrl.Log,
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	err = (&controllers.OpniDemoReconciler{
		Client: k8sClient,
		Log:    ctrl.Log,
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).NotTo(HaveOccurred())
	}()

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnicluster-test",
		},
	})
	Expect(err).Should(Or(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnidemo-test",
		},
	})
	Expect(err).Should(Or(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))
})

var _ = AfterSuite(func() {
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
	deleteTestClusterIfExists()
})
