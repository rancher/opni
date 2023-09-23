package cortex

import (
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
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

func workloadComponent(o metav1.Object) (value string, ok bool) {
	l := o.GetLabels()
	value, ok = l["app.kubernetes.io/component"]
	return
}

type CortexWorkloadOptions struct {
	replicas                      int32
	ports                         []Port
	extraArgs                     []string
	extraVolumes                  []corev1.Volume
	extraVolumeMounts             []corev1.VolumeMount
	extraEnvVars                  []corev1.EnvVar
	extraAnnotations              map[string]string
	sidecarContainers             []corev1.Container
	initContainers                []corev1.Container
	lifecycle                     *corev1.Lifecycle
	serviceName                   string
	deploymentStrategy            appsv1.DeploymentStrategy
	updateStrategy                appsv1.StatefulSetUpdateStrategy
	securityContext               corev1.SecurityContext
	affinity                      corev1.Affinity
	noLivenessProbe               bool
	noStartupProbe                bool
	terminationGracePeriodSeconds int64
	volumeClaimTemplates          []corev1.PersistentVolumeClaim
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

func ExtraEnvVars(envVars ...corev1.EnvVar) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.extraEnvVars = append(o.extraEnvVars, envVars...)
	}
}

func ExtraAnnotations(annotations map[string]string) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.extraAnnotations = annotations
	}
}

func SidecarContainers(containers ...corev1.Container) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.sidecarContainers = append(o.sidecarContainers, containers...)
	}
}

func InitContainers(containers ...corev1.Container) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.initContainers = append(o.initContainers, containers...)
	}
}

func DeploymentStrategy(strategy *appsv1.DeploymentStrategy) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.deploymentStrategy = *strategy
	}
}

func UpdateStrategy(strategy *appsv1.StatefulSetUpdateStrategy) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.updateStrategy = *strategy
	}
}

func SecurityContext(securityContext *corev1.SecurityContext) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.securityContext = *securityContext
	}
}

func Affinity(affinity *corev1.Affinity) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.affinity = *affinity
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

func NoStartupProbe() CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.noStartupProbe = true
	}
}

func NoLivenessProbe() CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.noLivenessProbe = true
	}
}

func TerminationGracePeriodSeconds(seconds int64) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.terminationGracePeriodSeconds = seconds
	}
}

func VolumeClaimTemplates(templates ...corev1.PersistentVolumeClaim) CortexWorkloadOption {
	return func(o *CortexWorkloadOptions) {
		o.volumeClaimTemplates = templates
	}
}

func (r *Reconciler) defaultWorkloadOptions(target string) CortexWorkloadOptions {
	options := CortexWorkloadOptions{
		replicas:    0,
		ports:       []Port{HTTP, Gossip, GRPC},
		serviceName: fmt.Sprintf("cortex-%s", target),
		securityContext: corev1.SecurityContext{
			ReadOnlyRootFilesystem: lo.ToPtr(true),
		},
		deploymentStrategy: appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       lo.ToPtr(intstr.FromInt(0)),
				MaxUnavailable: lo.ToPtr(intstr.FromInt(1)),
			},
		},
		updateStrategy: appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		},
		affinity: corev1.Affinity{
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
		terminationGracePeriodSeconds: 30,
	}

	return options
}

func (r *Reconciler) buildCortexDeploymentMeta(target string) *appsv1.Deployment {
	labels := cortexWorkloadLabels(target)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cortex-%s", target),
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}
}

func (r *Reconciler) buildCortexStatefulSetMeta(target string) *appsv1.StatefulSet {
	labels := cortexWorkloadLabels(target)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cortex-%s", target),
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}
}

func (r *Reconciler) buildCortexDeployment(
	target string,
	opts ...CortexWorkloadOption,
) *appsv1.Deployment {
	options := r.defaultWorkloadOptions(target)
	options.apply(opts...)

	labels := cortexWorkloadLabels(target)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cortex-%s", target),
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}

	dep.Spec = appsv1.DeploymentSpec{
		Replicas: &options.replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Strategy: options.deploymentStrategy,
		Template: r.cortexWorkloadPodTemplate(target, options),
	}

	ctrl.SetControllerReference(r.mc, dep, r.client.Scheme())
	return dep
}

func (r *Reconciler) buildCortexStatefulSet(
	target string,
	opts ...CortexWorkloadOption,
) *appsv1.StatefulSet {
	options := r.defaultWorkloadOptions(target)
	options.apply(opts...)

	labels := cortexWorkloadLabels(target)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-" + target,
			Namespace: r.mc.Namespace,
			Labels:    labels,
		},
	}

	statefulSet.Spec = appsv1.StatefulSetSpec{
		Replicas: &options.replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		UpdateStrategy:       options.updateStrategy,
		ServiceName:          options.serviceName,
		Template:             r.cortexWorkloadPodTemplate(target, options),
		VolumeClaimTemplates: options.volumeClaimTemplates,
	}

	ctrl.SetControllerReference(r.mc, statefulSet, r.client.Scheme())
	return statefulSet
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
			Labels:      labels,
			Annotations: options.extraAnnotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName:            "cortex",
			InitContainers:                options.initContainers,
			TerminationGracePeriodSeconds: &options.terminationGracePeriodSeconds,
			Containers: append([]corev1.Container{
				{
					Name:            target,
					Image:           r.mc.Status.Image,
					ImagePullPolicy: r.mc.Status.ImagePullPolicy,
					Args: append([]string{
						"cortex",
						"-target=" + target,
						"-config.file=/etc/cortex/cortex.yaml",
					}, options.extraArgs...),
					Ports:           ports,
					StartupProbe:    mtlsProbe,
					LivenessProbe:   mtlsProbe,
					ReadinessProbe:  mtlsProbe,
					SecurityContext: &options.securityContext,
					Env:             options.extraEnvVars,
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
							Name:      "opni-gateway-client-cert",
							MountPath: "/run/gateway/certs/client",
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
			}, options.sidecarContainers...),
			Affinity: &options.affinity,
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
							SecretName: "cortex-serving-cert-keys",
							Items:      tlsSecretItems,
						},
					},
				},
				{
					Name: "opni-gateway-client-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "opni-gateway-client-cert",
							Items:      tlsSecretItems,
						},
					},
				},
				{
					Name: "server-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "cortex-serving-cert-keys",
							Items:      tlsSecretItems,
						},
					},
				},
				{
					Name: "etcd-client-certs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "etcd-client-cert-keys",
							Items:      tlsSecretItems,
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

	targetLabels := cortexWorkloadLabels(target)

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
			Labels:    targetLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: targetLabels,
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
				Labels:    targetLabels,
			},
			Spec: corev1.ServiceSpec{
				Selector:                 targetLabels,
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
				Labels:    targetLabels,
			},
			Spec: monitoringv1.ServiceMonitorSpec{
				NamespaceSelector: monitoringv1.NamespaceSelector{
					MatchNames: []string{r.mc.Namespace},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: targetLabels,
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
		ctrl.SetControllerReference(r.mc, sm, r.client.Scheme())
	}

	return services
}
