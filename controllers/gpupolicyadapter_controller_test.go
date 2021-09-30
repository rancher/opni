package controllers

import (
	"context"

	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1beta1 "github.com/rancher/opni/apis/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GpuPolicyAdapter Controller", func() {
	When("creating a GpuPolicyAdapter", func() {
		It("should create a ClusterPolicy", func() {
			adapter := &v1beta1.GpuPolicyAdapter{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v1beta1.GpuPolicyAdapterSpec{},
			}
			Expect(k8sClient.Create(context.Background(), adapter)).To(Succeed())
			Eventually(Object(&nvidiav1.ClusterPolicy{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			})).Should(ExistAnd(HaveOwner(adapter)))
		})
	})
})
