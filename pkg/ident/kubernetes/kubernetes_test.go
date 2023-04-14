package kubernetes_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/ident/kubernetes"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/util/waitctx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes", Ordered, Label("unit", "slow"), func() {
	var restConfig *rest.Config
	BeforeAll(func() {
		var err error
		ctx, ca := context.WithCancel(waitctx.Background())
		restConfig, _, err = testk8s.StartK8s(ctx, "../../../testbin/bin", nil)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx)
		})
	})
	It("should obtain a unique identifier from a kubernetes cluster", func() {
		ident.RegisterProvider("k8s-test", func() ident.Provider {
			return kubernetes.NewKubernetesProvider(kubernetes.WithRestConfig(restConfig))
		})
		provider, err := ident.GetProvider("k8s-test")
		Expect(err).NotTo(HaveOccurred())

		cl, err := client.New(restConfig, client.Options{})
		Expect(err).NotTo(HaveOccurred())

		ns := corev1.Namespace{}
		err = cl.Get(context.Background(), types.NamespacedName{
			Name: "kube-system",
		}, &ns)
		Expect(err).NotTo(HaveOccurred())

		id, err := provider.UniqueIdentifier(context.Background())
		Expect(err).NotTo(HaveOccurred())

		Expect(id).To(BeEquivalentTo(ns.ObjectMeta.UID))
	})

	It("should already have an in-cluster kubernetes ident provider", func() {
		Expect(func() {
			ident.GetProvider("kubernetes")
		}).To(PanicWith(ContainSubstring(rest.ErrNotInCluster.Error())))
	})
})
