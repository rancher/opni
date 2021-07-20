package providers

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni/apis/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func BuildLogging(adapter *v1beta1.LogAdapter) *loggingv1beta1.Logging {
	return &loggingv1beta1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s",
				adapter.GetName(),
				strings.ToLower(string(adapter.Spec.Provider)),
			),
			Namespace: adapter.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         adapter.APIVersion,
					Kind:               adapter.Kind,
					Name:               adapter.Name,
					UID:                adapter.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
		Spec: loggingv1beta1.LoggingSpec{
			ControlNamespace: adapter.GetNamespace(),
			FluentbitSpec:    adapter.Spec.FluentConfig.Fluentbit,
			FluentdSpec:      adapter.Spec.FluentConfig.Fluentd,
		},
	}
}

func BuildRootLogging(adapter *v1beta1.LogAdapter) *loggingv1beta1.Logging {
	return &loggingv1beta1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adapter.GetName(),
			Namespace: adapter.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         adapter.APIVersion,
					Kind:               adapter.Kind,
					Name:               adapter.Name,
					UID:                adapter.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
		Spec: loggingv1beta1.LoggingSpec{
			ControlNamespace: adapter.GetNamespace(),
			FluentbitSpec:    adapter.Spec.RootFluentConfig.Fluentbit,
			FluentdSpec:      adapter.Spec.RootFluentConfig.Fluentd,
		},
	}
}

var (
	fluentBitK3sTemplate = template.Must(template.New("fluentbitk3s").Parse(`
[SERVICE]
    Flush             1
    Grace             5
    Daemon            Off
    Log_Level         info
    Coro_Stack_Size   24576
[INPUT]
    Name              systemd
    Tag               k3s
    Path              {{ .Spec.K3S.LogPath }}
    Systemd_Filter    _SYSTEMD_UNIT=k3s.service
    Strip_Underscores On
[FILTER]
    Name              lua
    Match             *
    script            received_at.lua
    call              append
[OUTPUT]
    Name              forward
    Match             *
    Host              {{ .ObjectMeta.Name }}-fluentd.{{ .ObjectMeta.Namespace }}.svc
    Port              24240
    Retry_Limit       False
`))

	receivedAtLua = `
function append(tag, timestamp, record)
    new_record = record
    new_record["timestamp"] = os.date("!%Y-%m-%dT%TZ",t)
    return 1, timestamp, new_record
end
`
	fluentBitRKETemplate = template.Must(template.New("fluentbitrke").Parse(`
[SERVICE]
    Log_Level         {{ .Spec.RKE.LogLevel }}
    Parsers_File      parsers.conf
[INPUT]
    Tag               rke
    Name              tail
    Path_Key          filename
    Parser            docker
    DB                /tail-db/tail-containers-state.db
    Mem_Buf_Limit     5MB
    Path              /var/lib/rancher/rke/log/*.log
[OUTPUT]
    Name              forward
    Match             *
    Host              {{ .ObjectMeta.Name }}-fluentd.{{ .ObjectMeta.Namespace }}.svc
    Port              24240
    Retry_Limit       False
`))
	fluentBitRKE2Template = template.Must(template.New("fluentbitrke2").Parse(`
[SERVICE]
    Flush             1
    Grace             5
    Daemon            Off
    Log_Level         info
    Coro_Stack_Size   24576
[INPUT]
    Name              systemd
    Tag               rke2
    Path              {{ .Spec.RKE2.LogPath }}
    Systemd_Filter    _SYSTEMD_UNIT=rke2-server.service
	Systemd_Filter    _SYSTEMD_UNIT=rke2-agent.service
    Strip_Underscores On
[FILTER]
    Name              lua
    Match             *
    script            received_at.lua
    call              append
[OUTPUT]
    Name              forward
    Match             *
    Host              {{ .ObjectMeta.Name }}-fluentd.{{ .ObjectMeta.Namespace }}.svc
    Port              24240
    Retry_Limit       False
`))
)

func BuildK3SConfig(adapter *v1beta1.LogAdapter) *corev1.ConfigMap {
	var buffer bytes.Buffer
	fluentBitK3sTemplate.Execute(&buffer, adapter)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-k3s", adapter.GetName()),
			Namespace: adapter.GetNamespace(),
		},
		Data: map[string]string{
			"fluent-bit.conf": buffer.String(),
			"received_at.lua": receivedAtLua,
		},
	}
}

func BuildK3SJournaldAggregator(adapter *v1beta1.LogAdapter) *appsv1.DaemonSet {
	name := fmt.Sprintf("%s-k3s-journald-aggregator", adapter.GetName())
	podLabels := map[string]string{
		"name": name,
	}
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: adapter.GetNamespace(),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: adapter.GetNamespace(),
					Labels:    podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "fluentbit",
							Image: adapter.Spec.FluentConfig.Fluentbit.Image.RepositoryWithTag(),
							SecurityContext: &corev1.SecurityContext{
								SELinuxOptions: &corev1.SELinuxOptions{
									Type: "rke_logreader_t",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/fluent-bit/etc/",
								},
								{
									Name:      "journal",
									MountPath: adapter.Spec.K3S.LogPath,
									ReadOnly:  true,
								},
								{
									Name:      "machine-id",
									MountPath: "/etc/machine-id",
									ReadOnly:  true,
								},
							},
						},
					},
					ServiceAccountName: name,
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: adapter.GetName() + "-k3s",
									},
								},
							},
						},
						{
							Name: "journal",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: adapter.Spec.K3S.LogPath,
								},
							},
						},
						{
							Name: "machine-id",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/etc/machine-id",
								},
							},
						},
					},
				},
			},
		},
	}
}

func BuildK3SServiceAccount(adapter *v1beta1.LogAdapter) *corev1.ServiceAccount {
	name := fmt.Sprintf("%s-k3s-journald-aggregator", adapter.GetName())
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: adapter.GetNamespace(),
		},
	}
}

func BuildRKEConfig(adapter *v1beta1.LogAdapter) *corev1.ConfigMap {
	var buffer bytes.Buffer
	fluentBitRKETemplate.Execute(&buffer, adapter)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-rke", adapter.GetName()),
			Namespace: adapter.GetNamespace(),
		},
		Data: map[string]string{
			"fluent-bit.conf": buffer.String(),
		},
	}
}

func BuildRKEAggregator(adapter *v1beta1.LogAdapter) *appsv1.DaemonSet {
	name := fmt.Sprintf("%s-rke-aggregator", adapter.GetName())
	podLabels := map[string]string{
		"name": name,
	}
	directoryOrCreate := corev1.HostPathDirectoryOrCreate
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: adapter.GetNamespace(),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: adapter.GetNamespace(),
					Labels:    podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "fluentbit",
							Image: adapter.Spec.FluentConfig.Fluentbit.Image.RepositoryWithTag(),
							SecurityContext: &corev1.SecurityContext{
								SELinuxOptions: &corev1.SELinuxOptions{
									Type: "rke_logreader_t",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath: "/var/lib/rancher/rke/log/",
									Name:      "indir",
								},
								{
									MountPath: adapter.Spec.ContainerLogDir,
									Name:      "containers",
								},
								{
									MountPath: "/tail-db",
									Name:      "positiondb",
								},
								{
									MountPath: "/fluent-bit/etc/fluent-bit.conf",
									Name:      "config",
									SubPath:   "fluent-bit.conf",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "indir",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/rancher/rke/log/",
									Type: &directoryOrCreate,
								},
							},
						},
						{
							Name: "containers",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: adapter.Spec.ContainerLogDir,
									Type: &directoryOrCreate,
								},
							},
						},
						{
							Name: "positiondb",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-rke", adapter.GetName()),
									},
								},
							},
						},
					},
					ServiceAccountName: name,
					Tolerations: []corev1.Toleration{
						{
							Key:    "node-role.kubernetes.io/controlplane",
							Value:  "true",
							Effect: corev1.TaintEffectNoSchedule,
						},
						{
							Key:    "node-role.kubernetes.io/etcd",
							Value:  "true",
							Effect: corev1.TaintEffectNoExecute,
						},
					},
				},
			},
		},
	}
}

func BuildRKEServiceAccount(adapter *v1beta1.LogAdapter) *corev1.ServiceAccount {
	name := fmt.Sprintf("%s-rke-aggregator", adapter.GetName())
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: adapter.GetNamespace(),
		},
	}
}

func BuildRKE2Config(adapter *v1beta1.LogAdapter) *corev1.ConfigMap {
	var buffer bytes.Buffer
	fluentBitRKE2Template.Execute(&buffer, adapter)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-rke2", adapter.GetName()),
			Namespace: adapter.GetNamespace(),
		},
		Data: map[string]string{
			"fluent-bit.conf": buffer.String(),
			"received_at.lua": receivedAtLua,
		},
	}
}
