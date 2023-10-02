package kubernetes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testk8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	namespace  = "oci-test"
)

func TestAPIs(t *testing.T) {
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultConsistentlyDuration(2 * time.Second)
	SetDefaultConsistentlyPollingInterval(100 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCI Kubernetes Handler Suite")
}

var _ = BeforeSuite(func() {
	ctx, ca := context.WithCancel(context.Background())
	var err error

	restConfig, scheme, err = testk8s.StartK8s(ctx, []string{
		"../../../config/crd/bases",
	}, apis.NewScheme())
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	k8sManager = testk8s.StartManager(ctx, restConfig, scheme)

	err = k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(ca)
})
