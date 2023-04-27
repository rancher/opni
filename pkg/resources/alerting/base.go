package alerting

import (
	"fmt"
	"net"
	"strings"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const dataMountPath = "/var/lib"

func alertingLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "opni-alerting",
	}
}

func (r *Reconciler) alerting() []resources.Resource {
	labelWithAlert := func(label map[string]string) map[string]string {
		label["app.kubernetes.io/name"] = "opni-alerting"
		return label
	}
	labelWithNode := func(label map[string]string) map[string]string {
		label["app.alerting.type"] = "node"
		return label
	}
	labelWithCluster := func(label map[string]string) map[string]string {
		label["app.alerting.node"] = "controller"
		return label
	}
	publicNodeLabels := labelWithNode(labelWithAlert(map[string]string{}))
	publicNodeSvcLabels := publicNodeLabels
	publicControllerLabels := labelWithCluster(labelWithAlert(map[string]string{}))
	publicControllerSvcLabels := publicControllerLabels
	requiredVolumes := []corev1.Volume{
		{
			Name: "opni-alertmanager-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: "opni-alertmanager-data",
					ReadOnly:  false,
				},
			},
		},
	}
	requiredPersistentClaims := []corev1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "opni-alertmanager-data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: util.Must(
							resource.ParseQuantity(fmt.Sprintf("%dGi", 5)),
						),
					},
				},
			},
		},
	}
	controllerService, controllerWorkers := r.newAlertingCluster(
		shared.OperatorAlertingControllerServiceName,
		r.alertmanagerControllerArgs(),
		publicControllerSvcLabels,
		publicControllerLabels,
		r.controllerAlertingPorts(),
		requiredVolumes,
		requiredPersistentClaims,
		1,
	)

	workerService, workerWorkers := r.newAlertingCluster(
		shared.OperatorAlertingClusterNodeServiceName,
		r.alertmanagerWorkerArgs(),
		publicNodeSvcLabels,
		publicNodeLabels,
		r.nodeAlertingPorts(),
		requiredVolumes,
		requiredPersistentClaims,
		lo.FromPtrOr(r.ac.Spec.Alertmanager.ApplicationSpec.Replicas, 1)-1,
	)
	ctrl.SetControllerReference(r.ac, controllerService, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, controllerWorkers, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, workerService, r.client.Scheme())
	ctrl.SetControllerReference(r.ac, workerWorkers, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, controllerService),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, controllerWorkers),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, workerService),
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable &&
			lo.FromPtrOr(r.ac.Spec.Alertmanager.ApplicationSpec.Replicas, 1) > 1,
			workerWorkers,
		),
	}
}

func (r *Reconciler) alertmanagerControllerArgs() []string {
	base := []string{
		fmt.Sprintf("--cluster.listen-address=0.0.0.0:%d", 9094),
		fmt.Sprintf("--config.file=%s", r.configPath()),
		fmt.Sprintf("--storage.path=%s", dataMountPath),
		fmt.Sprintf("--log.level=%s", "debug"),
		fmt.Sprintf("--opni.listen-address=:%d", 3000),
		"--log.format=json",
	}

	base = append(
		base,
		r.ac.Spec.Alertmanager.ApplicationSpec.ExtraArgs...,
	)
	return base
}

func (r *Reconciler) alertmanagerWorkerArgs() []string {
	base := []string{
		fmt.Sprintf("--config.file=%s", r.configPath()),
		fmt.Sprintf("--storage.path=%s", dataMountPath),
		fmt.Sprintf("--log.level=%s", "info"),
		"--log.format=json",
		// join to controller
		fmt.Sprintf("--cluster.peer=%s:%d",
			shared.OperatorAlertingControllerServiceName,
			9094,
		),
		fmt.Sprintf("--opni.listen-address=:%d", 3000),
	}
	base = append(
		base,
		r.ac.Spec.Alertmanager.ApplicationSpec.ExtraArgs...,
	)
	return base
}

func (r *Reconciler) syncerArgs() []string {
	_, gatewayPort, _ := net.SplitHostPort(strings.TrimPrefix(
		r.gw.Spec.Management.GetGRPCListenAddress(), "tcp://"))
	return []string{
		fmt.Sprintf("--syncer.alertmanager.config.file=%s", r.configPath()),
		fmt.Sprintf("--syncer.listen.address=:%d", 4000),
		fmt.Sprintf("--syncer.alertmanager.address=%s", fmt.Sprintf("http://localhost:%d", 9093)),
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
				Value: "alerting",
			},
		},
		Name:            "opni-alertmanager",
		Image:           r.gw.Status.Image,
		ImagePullPolicy: "Always",
		Args: append([]string{
			"alerting-server",
			"alertmanager",
		}, args...),
		Ports: ports,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "opni-alertmanager-data",
				MountPath: dataMountPath,
			},
			// // volume mount for alertmanager configmap
			// {
			// 	Name:      "opni-alertmanager-config",
			// 	MountPath: r.gw.Spec.Alerting.ConfigMountPath,
			// },
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/-/ready",
					Port: intstr.FromString("web-port"),
				},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/-/healthy",
					Port: intstr.FromString("web-port"),
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
				Value: "syncer",
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
		Ports: r.syncerPorts(),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "opni-alertmanager-data",
				MountPath: dataMountPath,
			},
		},
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

func (r *Reconciler) newAlertingCluster(
	name string,
	containerArgs []string,
	serviceLabels map[string]string,
	deployLabels map[string]string,
	alertManagerPorts []corev1.ContainerPort,
	volumes []corev1.Volume,
	persistentClaims []corev1.PersistentVolumeClaim,
	replicas int32,
) (*corev1.Service, *appsv1.StatefulSet) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.gw.Namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     "ClusterIP",
			Selector: deployLabels,
			Ports:    r.serviceAlertManagerPorts(append(alertManagerPorts, r.syncerPorts()...)),
		},
	}

	spec := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-internal",
			Namespace: r.gw.Namespace,
			Labels:    deployLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: deployLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: deployLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{},
					Containers: r.newAlertingClusterUnit(
						containerArgs,
						alertManagerPorts,
					),
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: persistentClaims,
		},
	}
	return svc, spec
}

func (r *Reconciler) controllerAlertingPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "opni-port",
			ContainerPort: shared.AlertingDefaultHookPort,
			Protocol:      "TCP",
		},
		{
			Name:          "web-port",
			ContainerPort: 9093,
			Protocol:      "TCP",
		},
		{
			Name:          "cluster-port",
			ContainerPort: 9094,
			Protocol:      "TCP",
		},
	}
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
			Protocol:      "TCP",
		},
		{
			Name:          "web-port",
			ContainerPort: 9093,
			Protocol:      "TCP",
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
