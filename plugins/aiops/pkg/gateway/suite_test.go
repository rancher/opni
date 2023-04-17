package gateway_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/util/waitctx"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	k8sClient  client.Client
	restConfig *rest.Config
	scheme     *runtime.Scheme
	k8sManager ctrl.Manager
)

func TestAPIs(t *testing.T) {
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Plugin Suite")
}

var _ = BeforeSuite(func() {
	var err error
	ctx, ca := context.WithCancel(waitctx.Background())
	restConfig, scheme, err = testk8s.StartK8s(ctx, []string{
		"../../../../config/crd/bases",
		"../../../../config/crd/opensearch",
		"../../../../test/resources",
	})
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		ca()
		waitctx.Wait(ctx)
	})

	k8sClient, err = client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	k8sManager = testk8s.StartManager(ctx, restConfig, scheme)
})
