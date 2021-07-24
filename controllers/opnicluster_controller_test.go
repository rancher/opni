package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/opni/apis/v1beta1"
)

var _ = FDescribe("OpniCluster Controller", func() {
	When("creating an opnicluster ", func() {
		cluster := v1beta1.OpniCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: crNamespace,
			},
			Spec: v1beta1.OpniClusterSpec{},
		}
		It("should succeed", func() {
			err := k8sClient.Create(context.Background(), &cluster)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should create the drain service deployment", func() {})
		It("should create the inference service deployment", func() {})
		It("should create the preprocessing service deployment", func() {})
		It("should create the payload receiver service deployment", func() {})
		It("should apply the correct labels to services", func() {})
		It("should not create any pretrained model services yet", func() {})
	})
	When("deleting an opnicluster", func() {
		It("should succeed", func() {})
		It("should delete all deployments", func() {})
	})
	When("creating a pretrained model", func() {
		// Not testing that the pretrained model controller works here, as that
		// is tested in the pretrained model controller test.
		It("should succeed", func() {})
	})
	When("referencing the pretrained model in an opnicluster", func() {
		It("should succeed", func() {})
		It("should create an inference service for the pretrained model", func() {})
	})
	Context("pretrained models should function in various configurations", func() {
		It("should work with multiple copies of a model specified", func() {})
		It("should work with multiple different models", func() {})
		It("should work with models with different source configurations", func() {})
	})
	When("adding pretrained models to an existing opnicluster", func() {
		It("should succeed", func() {})
		It("should create an inference service for the pretrained model", func() {})
	})
	When("deleting a pretrained model from an existing opnicluster", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
	})
	When("deleting an opnicluster with a pretrained model", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
		It("should keep the pretrainedmodel resource", func() {})
	})
	When("creating an opnicluster with an invalid pretrained model", func() {
		It("should wait and have a status condition", func() {})
		It("should resolve when the pretrained model is created", func() {})
	})
	When("deleting a pretrained model while an opnicluster is using it", func() {
		It("should succeed", func() {})
		It("should delete the inference service", func() {})
		It("should delete the pretrainedmodel resource", func() {})
		It("should cause the opnicluster to report a status condition", func() {})
	})
	When("deleting an opnicluster with a model that is also being used by another opnicluster", func() {
		It("should succeed", func() {})
		It("should delete the inference service only for the deleted opnicluster", func() {})
		It("should not delete the pretrainedmodel resource", func() {})
		It("should not cause the remaining opnicluster to report a status condition", func() {})
	})
})
