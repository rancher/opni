package alerting

import (
	"fmt"
	"net"
	"path"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) alerting() ([]resources.Resource, error) {
	// tls config
	webCfg, err := r.webConfig()
	if err != nil {
		return nil, err
	}

	labelWithAlerting := func(label map[string]string) map[string]string {
		label["app.kubernetes.io/name"] = "opni-alerting"
		return label
	}
	publicNodeLabels := labelWithAlerting(map[string]string{
		resources.PartOfLabel: "opni",
	})
	publicNodeSvcLabels := publicNodeLabels
	requiredPersistentClaims, requiredVolumes := r.storage()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.AlertmanagerService,
			Namespace: r.gw.Namespace,
			Labels:    publicNodeSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     "ClusterIP",
			Selector: publicNodeLabels,
			Ports:    r.serviceAlertManagerPorts(append(r.nodeAlertingPorts(), r.syncerPorts()...)),
		},
	}

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.AlertmanagerService,
			Namespace: r.gw.Namespace,
			Labels:    publicNodeLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: shared.AlertmanagerService,
			Replicas:    r.ac.Spec.Alertmanager.ApplicationSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: publicNodeLabels,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: publicNodeLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{},
					Containers: r.newAlertingClusterUnit(
						r.alertmanagerWorkerArgs(),
						r.nodeAlertingPorts(),
					),
					Volumes:                      requiredVolumes,
					ServiceAccountName:           "opni",
					AutomountServiceAccountToken: lo.ToPtr(true),
				},
			},
			VolumeClaimTemplates: requiredPersistentClaims,
		},
	}

	workerMonitor := r.serviceMonitor(
		shared.AlertmanagerService,
		publicNodeSvcLabels,
		[]monitoringv1.Endpoint{
			{
				Scheme:     "https",
				Path:       "/metrics",
				TargetPort: lo.ToPtr(intstr.FromString("web-port")),
			},
		},
	)

	oldSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: r.gw.Namespace,
		},
	}
	oldSS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: r.gw.Namespace,
		},
	}
	ctrl.SetControllerReference(r.ac, svc, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, ss, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, workerMonitor, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, oldSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, oldSS, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, webCfg, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, svc),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, ss),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, workerMonitor),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, webCfg),
		resources.Absent(oldSvc),
		resources.Absent(oldSS),
	}, nil
}

func (r *Reconciler) alertmanagerWorkerArgs() []string {
	amArgs := []string{
		fmt.Sprintf("--config.file=%s", r.configPath()),
		fmt.Sprintf("--storage.path=%s", dataMountPath),
		fmt.Sprintf("--log.level=%s", "debug"),
		"--log.format=json",
		fmt.Sprintf("--opni.listen-address=:%d", 3000),
		fmt.Sprintf("--web.config.file=%s", path.Join(webMountPath, "web.yml")),
		// TODO : make this configurable
		"--opni.send-k8s",
	}
	replicas := lo.FromPtrOr(r.ac.Spec.Alertmanager.ApplicationSpec.Replicas, 1)
	for i := 0; i < int(replicas); i++ {
		peerDomain := shared.AlertmanagerService
		amArgs = append(amArgs, fmt.Sprintf("--cluster.peer=%s-%d.%s:9094", shared.AlertmanagerService, i, peerDomain))
	}
	amArgs = append(
		amArgs,
		r.ac.Spec.Alertmanager.ApplicationSpec.ExtraArgs...,
	)
	return amArgs
}

func (r *Reconciler) syncerArgs() []string {
	_, gatewayPort, _ := net.SplitHostPort(strings.TrimPrefix(
		r.gw.Spec.Management.GetGRPCListenAddress(), "tcp://"))
	return []string{
		fmt.Sprintf("--syncer.alertmanager.config.file=%s", r.configPath()),
		fmt.Sprintf("--syncer.listen.address=:%d", 4000),
		fmt.Sprintf("--syncer.alertmanager.address=%s", fmt.Sprintf("localhost:%d", 9093)),
		fmt.Sprintf("--syncer.gateway.join.address=opni-internal:%s", gatewayPort),
	}
}

func (r *Reconciler) configPath() string {
	return fmt.Sprintf("%s/%s", dataMountPath, shared.AlertManagerConfigKey)
}

func (r *Reconciler) newAlertingAlertManager(
	args []string,
	ports []corev1.ContainerPort,
) corev1.Container {
	spec := corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name:  "USER",
				Value: alertingUser,
			},
			{
				Name:  "POD_NAMESPACE",
				Value: r.gw.Namespace,
			},
		},
		Name:            "opni-alertmanager",
		Image:           r.gw.Status.Image,
		ImagePullPolicy: "Always",
		Args: append([]string{
			"alerting-server",
			"alertmanager",
		}, args...),
		Ports:        ports,
		VolumeMounts: r.alertmanagerMounts(),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Scheme: corev1.URISchemeHTTPS,
					Path:   "/-/ready",
					Port:   intstr.FromString("web-port"),
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			FailureThreshold: 100,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Scheme: corev1.URISchemeHTTPS,
					Path:   "/-/healthy",
					Port:   intstr.FromString("web-port"),
				},
			},
		},
		Resources: lo.FromPtrOr(
			r.ac.Spec.Alertmanager.ApplicationSpec.ResourceRequirements,
			corev1.ResourceRequirements{},
		),
	}
	spec.Env = append(spec.Env, r.ac.Spec.Alertmanager.ApplicationSpec.ExtraEnvVars...)
	return spec
}

func (r *Reconciler) newAlertingSyncer(args []string) corev1.Container {
	spec := corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name:  "USER",
				Value: alertingUser,
			},
		},

		Name:            "opni-syncer",
		Image:           r.gw.Status.Image,
		ImagePullPolicy: "Always",
		Args: append([]string{
			"alerting-server",
			"syncer",
		},
			args...),
		Ports:        r.syncerPorts(),
		VolumeMounts: r.syncerMounts(),
	}

	spec.Env = append(spec.Env, r.ac.Spec.Alertmanager.ApplicationSpec.ExtraEnvVars...)
	return spec
}

func (r *Reconciler) newAlertingClusterUnit(
	alertmanagerArgs []string,
	alertManagerPorts []corev1.ContainerPort,
) []corev1.Container {
	res := []corev1.Container{
		r.newAlertingAlertManager(alertmanagerArgs, alertManagerPorts),
		r.newAlertingSyncer(r.syncerArgs()),
	}
	return res
}

func (r *Reconciler) syncerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "syncer-port",
			ContainerPort: 8080,
			Protocol:      "TCP",
		},
	}
}

func (r *Reconciler) nodeAlertingPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "opni-port",
			ContainerPort: shared.AlertingDefaultHookPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "web-port",
			ContainerPort: 9093,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "mesh-tcp",
			ContainerPort: 9094,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "mesh-udp",
			ContainerPort: 9094,
			Protocol:      corev1.ProtocolUDP,
		},
	}
}

func (r *Reconciler) serviceAlertManagerPorts(
	containerPorts []corev1.ContainerPort) []corev1.ServicePort {
	svcAMPorts := make([]corev1.ServicePort, len(containerPorts))
	for i, port := range containerPorts {
		svcAMPorts[i] = corev1.ServicePort{
			Name:       port.Name,
			Port:       port.ContainerPort,
			Protocol:   port.Protocol,
			TargetPort: intstr.FromString(port.Name),
		}
	}
	return svcAMPorts
}
