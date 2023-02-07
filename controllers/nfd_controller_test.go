package controllers

import (
	"context"

	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	nfdv1 "sigs.k8s.io/node-feature-discovery-operator/api/v1"
)

var _ = Describe("NfdController", Ordered, Label("controller", "deprecated"), func() {
	When("creating a NodeFeatureDiscovery", func() {
		It("should succeed", func() {
			nfd := &nfdv1.NodeFeatureDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(context.Background(), nfd)).To(Succeed())
			Eventually(Object(nfd)).Should(Exist())
		})
	})
})
