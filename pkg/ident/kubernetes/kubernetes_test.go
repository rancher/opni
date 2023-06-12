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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes", Ordered, Label("integration", "slow"), func() {
	var restConfig *rest.Config
	BeforeAll(func() {
		var err error
		ctx, ca := context.WithCancel(waitctx.Background())
		restConfig, _, err = testk8s.StartK8s(ctx, nil, scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx)
		})
	})
	It("should obtain a unique identifier from a kubernetes cluster", func() {
		ident.RegisterProvider("k8s-test", func(_ ...any) ident.Provider {
			return kubernetes.NewKubernetesProvider(kubernetes.WithRestConfig(restConfig))
		})
		builder, err := ident.GetProviderBuilder("k8s-test")
		Expect(err).NotTo(HaveOccurred())

		provider := builder()

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
			builder, _ := ident.GetProviderBuilder("kubernetes")
			builder()
		}).To(PanicWith(ContainSubstring(rest.ErrNotInCluster.Error())))
	})
})
