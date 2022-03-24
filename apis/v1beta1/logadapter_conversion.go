package v1beta1

import (
	"github.com/rancher/opni/apis/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *LogAdapter) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.LogAdapter)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Provider = v1beta2.LogProvider(src.Spec.Provider)
	dst.Spec.ContainerLogDir = src.Spec.ContainerLogDir
	dst.Spec.SELinuxEnabled = src.Spec.SELinuxEnabled

	if src.Spec.OpniCluster != nil {
		opniCluster := v1beta2.OpniClusterNameSpec(*src.Spec.OpniCluster)
		dst.Spec.OpniCluster = &opniCluster
	}

	if src.Spec.FluentConfig != nil {
		fluentConfig := v1beta2.FluentConfigSpec(*src.Spec.FluentConfig)
		dst.Spec.FluentConfig = &fluentConfig
	}
	if src.Spec.RootFluentConfig != nil {
		fluentConfig := v1beta2.FluentConfigSpec(*src.Spec.RootFluentConfig)
		dst.Spec.RootFluentConfig = &fluentConfig
	}

	if src.Spec.AKS != nil {
		aks := v1beta2.AKSSpec(*src.Spec.AKS)
		dst.Spec.AKS = &aks
	}
	if src.Spec.EKS != nil {
		eks := v1beta2.EKSSpec(*src.Spec.EKS)
		dst.Spec.EKS = &eks
	}
	if src.Spec.GKE != nil {
		gke := v1beta2.GKESpec(*src.Spec.GKE)
		dst.Spec.GKE = &gke
	}
	if src.Spec.K3S != nil {
		k3s := v1beta2.K3SSpec{
			ContainerEngine: v1beta2.ContainerEngine(src.Spec.K3S.ContainerEngine),
			LogPath:         src.Spec.K3S.LogPath,
		}
		dst.Spec.K3S = &k3s
	}
	if src.Spec.RKE2 != nil {
		rke2 := v1beta2.RKE2Spec(*src.Spec.RKE2)
		dst.Spec.RKE2 = &rke2
	}
	if src.Spec.RKE != nil {
		rke := v1beta2.RKESpec(*src.Spec.RKE)
		dst.Spec.RKE = &rke
	}

	return nil
}

//nolint:golint
func (dst *LogAdapter) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.LogAdapter)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Provider = LogProvider(src.Spec.Provider)
	dst.Spec.ContainerLogDir = src.Spec.ContainerLogDir
	dst.Spec.SELinuxEnabled = src.Spec.SELinuxEnabled

	if src.Spec.OpniCluster != nil {
		opniCluster := OpniClusterNameSpec(*src.Spec.OpniCluster)
		dst.Spec.OpniCluster = &opniCluster
	}

	if src.Spec.FluentConfig != nil {
		fluentConfig := FluentConfigSpec(*src.Spec.FluentConfig)
		dst.Spec.FluentConfig = &fluentConfig
	}
	if src.Spec.RootFluentConfig != nil {
		fluentConfig := FluentConfigSpec(*src.Spec.RootFluentConfig)
		dst.Spec.RootFluentConfig = &fluentConfig
	}

	if src.Spec.AKS != nil {
		aks := AKSSpec(*src.Spec.AKS)
		dst.Spec.AKS = &aks
	}
	if src.Spec.EKS != nil {
		eks := EKSSpec(*src.Spec.EKS)
		dst.Spec.EKS = &eks
	}
	if src.Spec.GKE != nil {
		gke := GKESpec(*src.Spec.GKE)
		dst.Spec.GKE = &gke
	}
	if src.Spec.K3S != nil {
		k3s := K3SSpec{
			ContainerEngine: ContainerEngine(src.Spec.K3S.ContainerEngine),
			LogPath:         src.Spec.K3S.LogPath,
		}
		dst.Spec.K3S = &k3s
	}
	if src.Spec.RKE2 != nil {
		rke2 := RKE2Spec(*src.Spec.RKE2)
		dst.Spec.RKE2 = &rke2
	}
	if src.Spec.RKE != nil {
		rke := RKESpec(*src.Spec.RKE)
		dst.Spec.RKE = &rke
	}

	return nil
}
