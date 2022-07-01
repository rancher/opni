package ident_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes", Ordered, Label("unit", "slow"), func() {
	var restConfig *rest.Config
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
		}
		var err error
		restConfig, err = env.StartK8s()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(env.Stop)
	})
	It("should obtain a unique identifier from a kubernetes cluster", func() {
		ident.RegisterProvider("k8s-test", func() ident.Provider {
			return ident.NewKubernetesProvider(ident.WithRestConfig(restConfig))
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
