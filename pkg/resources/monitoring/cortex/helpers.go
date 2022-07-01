package cortex

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Port string

const (
	HTTP   Port = "http"
	Gossip Port = "gossip"
	GRPC   Port = "grpc"
)

var (
	cortexAppLabel = map[string]string{
		"app.kubernetes.io/name": "cortex",
	}
)

func cortexWorkloadLabels(target string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "cortex",
		"app.kubernetes.io/part-of":   "opni",
		"app.kubernetes.io/instance":  "cortex",
		"app.kubernetes.io/component": target,
	}
}

type CortexWorkloadOptions struct {
	replicas          int32
	ports             []Port
	extraArgs         []string
	extraVolumes      []corev1.Volume
	extraVolumeMounts []corev1.VolumeMount
	lifecycle         *corev1.Lifecycle
	serviceName       string
	storageSize       string
}

type CortexWorkloadOption func(*CortexWorkloadOptions)

func (o *CortexWorkloadOptions) apply(opts ...CortexWorkloadOption) {
	for _, op := range opts {
		op(o)
	}
}

func Replicas(replicas int32) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.replicas = replicas
	}
}

func Ports(ports ...Port) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.ports = ports
	}
}

func ExtraArgs(args ...string) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.extraArgs = args
	}
}

func ExtraVolumes(volumes ...corev1.Volume) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.extraVolumes = volumes
	}
}

func ExtraVolumeMounts(volumeMounts ...corev1.VolumeMount) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.extraVolumeMounts = volumeMounts
	}
}

func Lifecycle(lifecycle *corev1.Lifecycle) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.lifecycle = lifecycle
	}
}

func ServiceName(serviceName string) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.serviceName = serviceName
	}
}

func StorageSize(storageSize string) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.storageSize = storageSize
	}
}

func NoPersistentStorage() CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.storageSize = ""
	}
}

func (r *Reconciler) buildCortexDeployment(
	target string,
	opts ...CortexWorkloadOption,
) resources.Resource {
	options := CortexWorkloadOptions{
		replicas: 3,
		ports:    []Port{HTTP, Gossip, GRPC},
	}
	options.apply(opts...)

	labels := cortexWorkloadLabels(target)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cortex-%s", target),
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}

	if !r.mc.Spec.Cortex.Enabled {
		return resources.Absent(dep)
	}

	dep.Spec = appsv1.DeploymentSpec{
		Replicas: &options.replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       util.Pointer(intstr.FromInt(0)),
				MaxUnavailable: util.Pointer(intstr.FromInt(1)),
			},
		},
		Template: r.cortexWorkloadPodTemplate(target, options),
	}

	ctrl.SetControllerReference(r.mc, dep, r.client.Scheme())
	return resources.Present(dep)
}

func (r *Reconciler) buildCortexStatefulSet(
	target string,
	opts ...CortexWorkloadOption,
) resources.Resource {
	options := CortexWorkloadOptions{
		replicas:    3,
		ports:       []Port{HTTP, Gossip, GRPC},
		serviceName: fmt.Sprintf("cortex-%s", target),
		storageSize: "2Gi",
	}
	options.apply(opts...)

	labels := cortexWorkloadLabels(target)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-" + target,
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}

	if !r.mc.Spec.Cortex.Enabled {
		return resources.Absent(statefulSet)
	}

	statefulSet.Spec = appsv1.StatefulSetSpec{
		Replicas: &options.replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		ServiceName: options.serviceName,
		Template:    r.cortexWorkloadPodTemplate(target, options),
	}

	if options.storageSize != "" {
		statefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "storage",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(options.storageSize),
						},
					},
				},
			},
		}
	}

	ctrl.SetControllerReference(r.mc, statefulSet, r.client.Scheme())
	return resources.PresentIff(r.mc.Spec.Cortex.Enabled, statefulSet)
}

func (r *Reconciler) cortexWorkloadPodTemplate(
	target string,
	options CortexWorkloadOptions,
) corev1.PodTemplateSpec {
	mtlsProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/usr/bin/curl", "-k", "https://127.0.0.1:8080/ready",
					"--key", "/run/cortex/certs/client/tls.key",
					"--cert", "/run/cortex/certs/client/tls.crt",
					"--cacert", "/run/cortex/certs/client/ca.crt",
				},
			},
		},
		InitialDelaySeconds: 5,
	}

	tlsSecretItems := []corev1.KeyToPath{
		{
			Key:  "tls.crt",
			Path: "tls.crt",
		},
		{
			Key:  "tls.key",
			Path: "tls.key",
		},
		{
			Key:  "ca.crt",
			Path: "ca.crt",
		},
	}

	ports := []corev1.ContainerPort{}
	for _, port := range options.ports {
		switch port {
		case HTTP:
			ports = append(ports, corev1.ContainerPort{
				Name:          "http-metrics",
				ContainerPort: 8080,
			})
		case Gossip:
			ports = append(ports, corev1.ContainerPort{
				Name:          "gossip",
				ContainerPort: 7946,
			})
		case GRPC:
			ports = append(ports, corev1.ContainerPort{
				Name:          "grpc",
				ContainerPort: 9095,
			})
		}
	}

	labels := cortexWorkloadLabels(target)

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "cortex",
			Containers: []corev1.Container{
				{
					Name:            target,
					Image:           r.mc.Status.Image,
					ImagePullPolicy: r.mc.Status.ImagePullPolicy,
					Command:         []string{"opni", "cortex"},
					Args: append([]string{
						"-target=" + target,
						"-config.file=/etc/cortex/cortex.yaml",
					}, options.extraArgs...),
					Ports:          ports,
					StartupProbe:   mtlsProbe,
					LivenessProbe:  mtlsProbe,
					ReadinessProbe: mtlsProbe,
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem: util.Pointer(true),
					},
					Env: r.mc.Spec.Cortex.ExtraEnvVars,
					VolumeMounts: append([]corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
						{
							Name:      "config",
							MountPath: "/etc/cortex",
						},
						{
							Name:      "runtime-config",
							MountPath: "/etc/cortex-runtime-config",
						},
						{
							Name:      "client-certs",
							MountPath: "/run/cortex/certs/client",
							ReadOnly:  true,
						},
						{
							Name:      "server-certs",
							MountPath: "/run/cortex/certs/server",
							ReadOnly:  true,
						},
						{
							Name:      "etcd-client-certs",
							MountPath: "/run/etcd/certs/client",
							ReadOnly:  true,
						},
						{
							Name:      "etcd-server-cacert",
							MountPath: "/run/etcd/certs/server",
						},
					}, options.extraVolumeMounts...),
					Lifecycle: options.lifecycle,
				},
			},
			Affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchExpressions: []metav1.LabelSelectorRequirement{
										{
											Key:      "app.kubernetes.io/component",
											Operator: metav1.LabelSelectorOpIn,
											Values:   []string{target},
										},
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
							Weight: 100,
						},
					},
				},
			},
			Volumes: append([]corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "cortex",
						},
					},
				},
				{
					Name: "runtime-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "cortex-runtime-config",
							},
						},
					},
				},
				{
					Name: "client-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "cortex-serving-cert-keys",
							Items:       tlsSecretItems,
							DefaultMode: util.Pointer[int32](0644),
						},
					},
				},
				{
					Name: "server-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "cortex-serving-cert-keys",
							Items:       tlsSecretItems,
							DefaultMode: util.Pointer[int32](0644),
						},
					},
				},
				{
					Name: "etcd-client-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  "etcd-client-cert-keys",
							Items:       tlsSecretItems,
							DefaultMode: util.Pointer[int32](0644),
						},
					},
				},
				{
					Name: "etcd-server-cacert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "etcd-serving-cert-keys",
							Items: []corev1.KeyToPath{
								{
									Key:  "ca.crt",
									Path: "ca.crt",
								},
							},
							DefaultMode: util.Pointer[int32](0644),
						},
					},
				},
			}, options.extraVolumes...),
		},
	}
}

type CortexServiceOptions struct {
	addHeadlessService       bool
	publishNotReadyAddresses bool
	addServiceMonitor        bool
}

type CortexServiceOption func(*CortexServiceOptions)

func (o *CortexServiceOptions) apply(opts ...CortexServiceOption) {
	for _, op := range opts {
		op(o)
	}
}

func AddHeadlessService(publishNotReadyAddresses bool) CortexServiceOption {
	return func(o *CortexServiceOptions) {
		o.addHeadlessService = true
		o.publishNotReadyAddresses = publishNotReadyAddresses
	}
}

func AddServiceMonitor() CortexServiceOption {
	return func(o *CortexServiceOptions) {
		o.addServiceMonitor = true
	}
}

func (r *Reconciler) buildCortexWorkloadServices(
	target string,
	opts ...CortexServiceOption,
) []resources.Resource {
	options := CortexServiceOptions{}
	options.apply(opts...)

	httpPort := corev1.ServicePort{
		Name:       "http-metrics",
		Port:       8080,
		TargetPort: intstr.FromString("http-metrics"),
	}

	grpcPort := corev1.ServicePort{
		Name:       "grpc",
		Port:       9095,
		TargetPort: intstr.FromString("grpc"),
	}

	services := []resources.Resource{}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-" + target,
			Namespace: r.mc.Namespace,
			Labels:    cortexWorkloadLabels(target),
		},
		Spec: corev1.ServiceSpec{
			Selector: cortexWorkloadLabels(target),
			Type:     corev1.ServiceTypeClusterIP,
			Ports:    []corev1.ServicePort{httpPort},
		},
	}
	services = append(services, resources.Present(svc))
	ctrl.SetControllerReference(r.mc, svc, r.client.Scheme())

	if options.addHeadlessService {
		headlessSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cortex-" + target + "-headless",
				Namespace: r.mc.Namespace,
				Labels:    cortexWorkloadLabels(target),
			},
			Spec: corev1.ServiceSpec{
				Selector:                 cortexWorkloadLabels(target),
				Type:                     corev1.ServiceTypeClusterIP,
				ClusterIP:                corev1.ClusterIPNone,
				Ports:                    []corev1.ServicePort{grpcPort},
				PublishNotReadyAddresses: options.publishNotReadyAddresses,
			},
		}
		services = append(services, resources.Present(headlessSvc))
		ctrl.SetControllerReference(r.mc, headlessSvc, r.client.Scheme())
	}

	if options.addServiceMonitor {
		sm := &monitoringv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cortex-" + target,
				Namespace: r.mc.Namespace,
				Labels:    cortexWorkloadLabels(target),
			},
			Spec: monitoringv1.ServiceMonitorSpec{
				NamespaceSelector: monitoringv1.NamespaceSelector{
					MatchNames: []string{r.mc.Namespace},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: cortexWorkloadLabels(target),
				},
				Endpoints: []monitoringv1.Endpoint{
					{
						Port:   "http-metrics",
						Path:   "/metrics",
						Scheme: "https",
						TLSConfig: &monitoringv1.TLSConfig{
							SafeTLSConfig: monitoringv1.SafeTLSConfig{
								CA: monitoringv1.SecretOrConfigMap{
									Secret: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cortex-client-cert-keys",
										},
										Key: "ca.crt",
									},
								},
								Cert: monitoringv1.SecretOrConfigMap{
									Secret: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "cortex-client-cert-keys",
										},
										Key: "tls.crt",
									},
								},
								KeySecret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "cortex-client-cert-keys",
									},
									Key: "tls.key",
								},
								ServerName: "cortex-server",
							},
						},
					},
				},
			},
		}
		services = append(services, resources.Present(sm))
		controllerutil.SetOwnerReference(r.mc, sm, r.client.Scheme())
	}

	return services
}
