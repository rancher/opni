package controllers

import (
	"context"

	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GpuPolicyAdapter Controller", Ordered, Label("controller"), func() {
	When("creating a GpuPolicyAdapter", func() {
		It("should create a ClusterPolicy", func() {
			adapter := &v1beta2.GpuPolicyAdapter{
				ObjectMeta: v1.ObjectMeta{
					Name: "test",
				},
				Spec: v1beta2.GpuPolicyAdapterSpec{},
			}
			Expect(k8sClient.Create(context.Background(), adapter)).To(Succeed())
			Eventually(Object(&nvidiav1.ClusterPolicy{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: adapter.Namespace,
				},
			})).Should(ExistAnd(HaveOwner(adapter)))
		})
	})
})
