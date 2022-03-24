package v1beta1

import (
	"github.com/rancher/opni/apis/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *GpuPolicyAdapter) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.GpuPolicyAdapter)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ContainerRuntime = v1beta2.ContainerRuntime(src.Spec.ContainerRuntime)
	dst.Spec.KubernetesProvider = src.Spec.KubernetesProvider
	dst.Spec.Images = v1beta2.ImagesSpec(src.Spec.Images)
	dst.Spec.Template = src.Spec.Template

	if src.Spec.VGPU != nil {
		vgpu := &v1beta2.VGPUSpec{
			LicenseConfigMap:  src.Spec.VGPU.LicenseConfigMap,
			LicenseServerKind: src.Spec.VGPU.LicenseServerKind,
		}
		dst.Spec.VGPU = vgpu
	}

	return nil
}

//nolint:golint
func (dst *GpuPolicyAdapter) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.GpuPolicyAdapter)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.ContainerRuntime = ContainerRuntime(src.Spec.ContainerRuntime)
	dst.Spec.KubernetesProvider = src.Spec.KubernetesProvider
	dst.Spec.Images = ImagesSpec(src.Spec.Images)
	dst.Spec.Template = src.Spec.Template

	if src.Spec.VGPU != nil {
		vgpu := &VGPUSpec{
			LicenseConfigMap:  src.Spec.VGPU.LicenseConfigMap,
			LicenseServerKind: src.Spec.VGPU.LicenseServerKind,
		}
		dst.Spec.VGPU = vgpu
	}

	return nil
}
