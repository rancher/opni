package logging

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	// all processes
	k8sClient  client.Client
	restConfig *rest.Config
	scheme     = apis.NewScheme()

	// process 1 only
	k8sManager ctrl.Manager
	testEnv    *envtest.Environment
	stopEnv    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Plugin Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(util.NewTestLogger())
	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		Scheme: scheme,
		CRDs:   test.DownloadCertManagerCRDs(scheme),
		CRDDirectoryPaths: []string{
			"../config/crd/bases",
			"../config/crd/opensearch",
			"../test/resources",
		},
		BinaryAssetsDirectory: "../testbin/bin",
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

	stopEnv, k8sManager, k8sClient = test.RunTestEnvironment(testEnv, false, false,
		&controllers.OpniOpensearchReconciler{},
	)

	restConfig = testEnv.Config

	DeferCleanup(func() {
		By("tearing down the test environment")
		stopEnv()
		test.ExternalResources.Wait()
	})
})
