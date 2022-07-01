package controllers

import (
	"context"

	. "github.com/kralicky/kmatch"
	nfdv1 "github.com/kubernetes-sigs/node-feature-discovery-operator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("NfdController", Ordered, Label("controller"), func() {
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
