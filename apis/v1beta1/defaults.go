package v1beta1

import (
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"k8s.io/utils/pointer"
)

func enableSecuritySettings(spec *LogAdapterSpec) {
	spec.Fluentbit.Security.PodSecurityPolicyCreate = true
	spec.Fluentbit.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
	spec.Fluentd.Security.PodSecurityPolicyCreate = true
	spec.Fluentd.Security.RoleBasedAccessControlCreate = pointer.Bool(true)
}

// ApplyDefaults will configure the default provider-specific settings, and
// apply any defaults for the Fluentbit or Fluentd specs if needed. This
// function will ensure the corresponding provider-specific spec field is
// not nil. When this function is called, a.Spec.Fluentbit and a.Spec.Fluentd
// are guaranteed not to be nil.
func (p LogProvider) ApplyDefaults(a *LogAdapter) {
	enableSecuritySettings(&a.Spec)

	switch p {
	case LogProviderAKS:
		if a.Spec.AKS == nil {
			a.Spec.AKS = &AKSSpec{}
		}
		a.Spec.Fluentbit.InputTail.Tag = "aks"
		a.Spec.Fluentbit.InputTail.Path = "/var/log/azure/kubelet-status.log"
	case LogProviderEKS:
		if a.Spec.EKS == nil {
			a.Spec.EKS = &EKSSpec{}
		}
		a.Spec.Fluentbit.InputTail.Tag = "eks"
		a.Spec.Fluentbit.InputTail.Path = "/var/log/messages"
		a.Spec.Fluentbit.InputTail.Parser = "syslog"
	case LogProviderGKE:
		if a.Spec.GKE == nil {
			a.Spec.GKE = &GKESpec{}
		}
		a.Spec.Fluentbit.InputTail.Tag = "gke"
		a.Spec.Fluentbit.InputTail.Path = "/var/log/kube-proxy.log"
	case LogProviderK3S:
		if a.Spec.K3S == nil {
			a.Spec.K3S = &K3SSpec{
				ContainerEngine: ContainerEngineSystemd,
			}
			if a.Spec.K3S.ContainerEngine == ContainerEngineSystemd {
				a.Spec.K3S.LogPath = "/var/log/journal"
			}
		}
		a.Spec.Fluentbit.InputTail.Tag = "k3s"
		if a.Spec.K3S.LogPath != "" {
			a.Spec.Fluentbit.InputTail.Path = a.Spec.K3S.LogPath
		}
		a.Spec.Fluentbit.InputTail.PathKey = "filename"
		a.Spec.Fluentbit.ExtraVolumeMounts = append(a.Spec.Fluentbit.ExtraVolumeMounts,
			&loggingv1beta1.VolumeMount{
				Source:      "/var/log",
				Destination: "/var/log",
				ReadOnly:    pointer.Bool(true),
			},
		)
	case LogProviderRKE:
		if a.Spec.RKE == nil {
			a.Spec.RKE = &RKESpec{}
		}
	case LogProviderRKE2:
		if a.Spec.RKE2 == nil {
			a.Spec.RKE2 = &RKE2Spec{}
		}
	case LogProviderKubeAudit:
		if a.Spec.KubeAudit == nil {
			a.Spec.KubeAudit = &KubeAuditSpec{
				PathPrefix:    "/var/log/kube-audit",
				AuditFilename: "/var/log/kube-audit/kube-audit.log",
			}
		}
		a.Spec.Fluentbit.DisableKubernetesFilter = pointer.Bool(true)
		a.Spec.Fluentbit.ExtraVolumeMounts = append(a.Spec.Fluentbit.ExtraVolumeMounts,
			&loggingv1beta1.VolumeMount{
				Source:      a.Spec.KubeAudit.PathPrefix,
				Destination: "/kube-audit-logs",
				ReadOnly:    pointer.Bool(true),
			},
		)
	}
}
