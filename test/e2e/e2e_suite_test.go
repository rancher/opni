package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Tests")
}

var (
	testEnv     *envtest.Environment
	stopEnv     context.CancelFunc
	k8sClient   crclient.Client
	restConfig  *rest.Config
	useExisting bool
)

var _ = BeforeSuite(func() {
	logf.SetLogger(util.NewTestLogger())
	logf.Log.Info("Using existing cluster")

	fmt.Println("KUBECONFIG=" + os.Getenv("KUBECONFIG"))

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
	stopEnv, mgr, k8sClient = test.RunTestEnvironment(testEnv, false, true)
	if restConfig == nil {
		restConfig = mgr.GetConfig()
	}
})

var _ = AfterSuite(func() {
	stopEnv()
})
