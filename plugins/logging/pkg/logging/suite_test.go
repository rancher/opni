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
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	k8sClient  client.Client
	restConfig *rest.Config
	scheme     = apis.NewScheme()
	k8sManager ctrl.Manager
	testEnv    *envtest.Environment
	stopEnv    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logging Plugin Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	logf.SetLogger(util.NewTestLogger())
	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		Scheme: scheme,
		CRDs:   test.DownloadCertManagerCRDs(scheme),
		CRDDirectoryPaths: []string{
			"../../../../config/crd/bases",
			"../../../../config/crd/opensearch",
			"../../../../test/resources",
		},
		BinaryAssetsDirectory: "../../../../testbin/bin",
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
	config := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"default": {
				Server:                   restConfig.Host,
				CertificateAuthorityData: restConfig.CAData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default": {
				ClientCertificateData: restConfig.CertData,
				ClientKeyData:         restConfig.KeyData,
				Username:              restConfig.Username,
				Password:              restConfig.Password,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default",
			},
		},
		CurrentContext: "default",
	}
	configBytes, err := clientcmd.Write(config)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		By("tearing down the test environment")
		stopEnv()
		test.ExternalResources.Wait()
	})

	return configBytes
}, func(configBytes []byte) {
	By("connecting to the test environment")
	if k8sClient != nil {
		return
	}
	config, err := clientcmd.Load(configBytes)
	Expect(err).NotTo(HaveOccurred())
	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	restConfig.QPS = 1000.0
	restConfig.Burst = 2000.0
	Expect(err).NotTo(HaveOccurred())
	k8sClient, err = client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	Expect(err).NotTo(HaveOccurred())
})
