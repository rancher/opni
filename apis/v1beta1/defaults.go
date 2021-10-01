package v1beta1

import (
	"path/filepath"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

var namespacedKubeURL = "https://kubernetes.default:443"

func enableSecuritySettings(spec *LogAdapterSpec) {
	if spec.FluentConfig.Fluentbit.Security == nil {
		spec.FluentConfig.Fluentbit.Security = &loggingv1beta1.Security{}
	}
	if spec.RootFluentConfig.Fluentbit.Security == nil {
		spec.RootFluentConfig.Fluentbit.Security = &loggingv1beta1.Security{}
	}
	if spec.FluentConfig.Fluentd.Security == nil {
		spec.FluentConfig.Fluentd.Security = &loggingv1beta1.Security{}
	}
	if spec.RootFluentConfig.Fluentd.Security == nil {
		spec.RootFluentConfig.Fluentd.Security = &loggingv1beta1.Security{}
	}

	spec.FluentConfig.Fluentbit.Security.PodSecurityPolicyCreate = true
	spec.RootFluentConfig.Fluentbit.Security.PodSecurityPolicyCreate = true
	spec.FluentConfig.Fluentbit.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
	spec.RootFluentConfig.Fluentbit.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
	spec.FluentConfig.Fluentd.Security.PodSecurityPolicyCreate = true
	spec.RootFluentConfig.Fluentd.Security.PodSecurityPolicyCreate = true
	spec.FluentConfig.Fluentd.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
	spec.RootFluentConfig.Fluentd.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
	if spec.SELinuxEnabled {
		spec.RootFluentConfig.Fluentbit.Security.SecurityContext = &v1.SecurityContext{
			SELinuxOptions: &v1.SELinuxOptions{
				Type: "rke_logreader_t",
			},
		}
	}
}

func setRootLoggingSettings(spec *LogAdapterSpec) {
	spec.RootFluentConfig.Fluentbit.MountPath = spec.ContainerLogDir
	if spec.ContainerLogDir != "/var/lib/docker/containers" {
		spec.RootFluentConfig.Fluentbit.ExtraVolumeMounts = appendOrUpdateVolumeMount(spec.RootFluentConfig.Fluentbit.ExtraVolumeMounts,
			&loggingv1beta1.VolumeMount{
				Source:      spec.ContainerLogDir,
				Destination: spec.ContainerLogDir,
				ReadOnly:    pointer.Bool(true),
			},
		)
	}
}

// ApplyDefaults will configure the default provider-specific settings, and
// apply any defaults for the Fluentbit or Fluentd specs if needed. This
// function will ensure the corresponding provider-specific spec field is
// not nil. When this function is called, a.Spec.Fluentbit and a.Spec.Fluentd
// are guaranteed not to be nil.
func (p LogProvider) ApplyDefaults(a *LogAdapter) {
	enableSecuritySettings(&a.Spec)
	setRootLoggingSettings(&a.Spec)

	switch p {
	case LogProviderAKS:
		if a.Spec.AKS == nil {
			a.Spec.AKS = &AKSSpec{}
		}
		a.Spec.FluentConfig.Fluentbit.InputTail.Tag = "aks"
		a.Spec.FluentConfig.Fluentbit.InputTail.Path = "/var/log/azure/kubelet-status.log"
	case LogProviderEKS:
		if a.Spec.EKS == nil {
			a.Spec.EKS = &EKSSpec{}
		}
		a.Spec.FluentConfig.Fluentbit.InputTail.Tag = "eks"
		a.Spec.FluentConfig.Fluentbit.InputTail.Path = "/var/log/messages"
		a.Spec.FluentConfig.Fluentbit.InputTail.Parser = "syslog"
	case LogProviderGKE:
		if a.Spec.GKE == nil {
			a.Spec.GKE = &GKESpec{}
		}
		a.Spec.FluentConfig.Fluentbit.InputTail.Tag = "gke"
		a.Spec.FluentConfig.Fluentbit.InputTail.Path = "/var/log/kube-proxy.log"
	case LogProviderK3S:
		if a.Spec.K3S == nil {
			a.Spec.K3S = &K3SSpec{
				ContainerEngine: ContainerEngineSystemd,
			}
		}
		if a.Spec.K3S.ContainerEngine == ContainerEngineSystemd && a.Spec.K3S.LogPath == "" {
			a.Spec.K3S.LogPath = "/var/log/journal"
		}
		if a.Spec.K3S.ContainerEngine == ContainerEngineOpenRC && a.Spec.K3S.LogPath == "" {
			a.Spec.K3S.LogPath = "/var/log/k3s.log"
		}
		logDir := filepath.Dir(a.Spec.K3S.LogPath)
		a.Spec.FluentConfig.Fluentbit.InputTail.Tag = "k3s"
		a.Spec.FluentConfig.Fluentbit.InputTail.Path = a.Spec.K3S.LogPath
		a.Spec.FluentConfig.Fluentbit.InputTail.PathKey = "filename"
		a.Spec.FluentConfig.Fluentbit.ExtraVolumeMounts = appendOrUpdateVolumeMount(a.Spec.FluentConfig.Fluentbit.ExtraVolumeMounts,
			&loggingv1beta1.VolumeMount{
				Source:      logDir,
				Destination: logDir,
				ReadOnly:    pointer.Bool(true),
			},
		)
		a.Spec.RootFluentConfig.Fluentbit.FilterKubernetes.KubeURL = namespacedKubeURL
	case LogProviderRKE:
		if a.Spec.RKE == nil {
			a.Spec.RKE = &RKESpec{}
			a.Spec.RKE.LogLevel = LogLevelInfo
		}
	case LogProviderRKE2:
		if a.Spec.RKE2 == nil {
			a.Spec.RKE2 = &RKE2Spec{}
			a.Spec.RKE2.LogPath = "/var/log/journal"
		}
		a.Spec.RootFluentConfig.Fluentbit.FilterKubernetes.KubeURL = namespacedKubeURL
	}
}

func appendOrUpdateVolumeMount(mounts []*loggingv1beta1.VolumeMount, newMount *loggingv1beta1.VolumeMount) []*loggingv1beta1.VolumeMount {
	for i, v := range mounts {
		if v.Destination == newMount.Destination {
			mounts[i] = newMount
			return mounts
		}
	}
	return append(mounts, newMount)
}
