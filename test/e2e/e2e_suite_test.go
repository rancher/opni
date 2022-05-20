package e2e

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Tests")
}

var (
	testEnv    *envtest.Environment
	stopEnv    context.CancelFunc
	k8sClient  crclient.Client
	restConfig *rest.Config
)

var _ = BeforeSuite(func() {
	// kubeconfig := os.Getenv("KUBECONFIG")
	// client, err := util.NewK8sClient(util.ClientOptions{
	// 	Kubeconfig: &kubeconfig,
	// })
	// if err != nil {
	// 	panic(err)
	// }
})
