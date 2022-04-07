package controllers_test

import (
	"testing"
	"time"

	"github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/sdk/controllers"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util/testutil"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(5 * time.Second)
	SetDefaultEventuallyPollingInterval(50 * time.Millisecond)
	SetDefaultConsistentlyDuration(1 * time.Second)
	SetDefaultConsistentlyPollingInterval(50 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite")
}

var (
	testEnv    *test.Environment
	restConfig *rest.Config
	k8sManager manager.Manager
	k8sClient  client.Client
)

var (
	reconcilers = []test.Reconciler{
		&controllers.GatewayReconciler{},
	}
)

var _ = BeforeSuite(func() {
	testEnv = &test.Environment{
		TestBin:           "../../../testbin/bin",
		Logger:            test.Log,
		CRDDirectoryPaths: []string{"../crd"},
	}
	restConfig = testutil.Must(testEnv.StartK8s())
	k8sManager = testutil.Must(testEnv.StartManager(restConfig, reconcilers...))
	k8sClient = k8sManager.GetClient()
	kmatch.SetDefaultObjectClient(k8sClient)
	DeferCleanup(testEnv.Stop)
})
