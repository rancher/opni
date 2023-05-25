package kubernetes_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/oci/kubernetes"
	"github.com/rancher/opni/pkg/versions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Kubernetes OCI handler", Ordered, Label("unit", "slow"), func() {
	var (
		gateway *corev1beta1.Gateway
		k8sOCI  oci.Fetcher
	)
	BeforeAll(func() {
		gateway = &corev1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-gateway",
				Namespace: namespace,
			},
			Spec: corev1beta1.GatewaySpec{},
		}
		Expect(k8sClient.Create(context.Background(), gateway)).To(Succeed())

		var err error
		k8sOCI, err = kubernetes.NewKubernetesResolveImageDriver(
			namespace,
			kubernetes.WithRestConfig(restConfig),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	When("an unknown image type is requested", func() {
		It("should return unsported image type error", func() {
			_, err := k8sOCI.GetImage(context.Background(), "unknown")
			Expect(err).To(MatchError(kubernetes.ErrUnsupportedImageType))
		})
	})

	When("gateway status is not set", func() {
		It("should not return opni image", func() {
			_, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeOpni)
			Expect(err).To(MatchError(kubernetes.ErrImageNotFound))
		})
		It("should not return the minimal image", func() {
			_, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeOpni)
			Expect(err).To(MatchError(kubernetes.ErrImageNotFound))
		})
		When("version is set", func() {
			It("should not return the minimal image", func() {
				versions.SetVersionForTest("v1.0.0")
				defer versions.UnlockVersionForTest()
				_, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeOpni)
				Expect(err).To(MatchError(kubernetes.ErrImageNotFound))
			})
		})
	})

	When("gateway status is set", Ordered, func() {
		When("version is unset", func() {
			It("should set the status", func() {
				gateway.Status = corev1beta1.GatewayStatus{
					Image: "rancher/opni-test@sha256:1234567890",
				}
				Expect(k8sClient.Status().Update(context.Background(), gateway)).To(Succeed())
			})
			It("should return the opni image", func() {
				image, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeOpni)
				Expect(err).NotTo(HaveOccurred())
				Expect(image.String()).To(Equal("rancher/opni-test@sha256:1234567890"))
			})
			It("should return the opni image as the minimal image", func() {
				versions.SetVersionForTest("unversioned")
				defer versions.UnlockVersionForTest()
				image, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeMinimal)
				Expect(err).NotTo(HaveOccurred())
				Expect(image.String()).To(Equal("rancher/opni-test@sha256:1234567890"))
			})
		})
		When("version is set", func() {
			It("should return the minimal image", func() {
				versions.SetVersionForTest("v1.0.0")
				defer versions.UnlockVersionForTest()
				image, err := k8sOCI.GetImage(context.Background(), oci.ImageTypeMinimal)
				Expect(err).NotTo(HaveOccurred())
				Expect(image.String()).To(Equal("rancher/opni-test:v1.0.0-minimal"))
			})
		})
	})
})
