package gateway

import (
	"fmt"
	"net"
	"strings"

	"github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/util/nats/k8snats"

	"github.com/rancher/opni/pkg/alerting/shared"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rancher/opni/pkg/resources"
)

func alertingMutator(spec *v1beta1.AlertingSpec) {
	// networking
	if spec.WebPort == 0 {
		spec.WebPort = 9093
	}
	if spec.ClusterPort == 0 {
		spec.ClusterPort = 9094
	}
	// resources
	if spec.Storage == "" {
		spec.Storage = "500Mi"
	}
	if spec.CPU == "" {
		spec.CPU = "500m"
	}
	if spec.Memory == "" {
		spec.Memory = "200Mi"
	}

	// cluster-behaviour
	if spec.Replicas == 0 {
		spec.Replicas = 1
	}
	if spec.ClusterSettleTimeout == "" {
		spec.ClusterSettleTimeout = "1m0s"
	}
	if spec.ClusterPushPullInterval == "" {
		spec.ClusterPushPullInterval = "1m0s"
	}
	if spec.ClusterGossipInterval == "" {
		spec.ClusterGossipInterval = "200ms"
	}
	if spec.DataMountPath == "" {
		spec.DataMountPath = shared.DataMountPath
	}
}

func (r *Reconciler) alertmanagerControllerArgs() []string {
	return []string{
		fmt.Sprintf("--cluster.listen-address=0.0.0.0:%d", r.gw.Spec.Alerting.ClusterPort),
		fmt.Sprintf("--config.file=%s", r.configPath()),
		fmt.Sprintf("--storage.path=%s", r.gw.Spec.Alerting.DataMountPath),
		fmt.Sprintf("--log.level=%s", "debug"),
		"--log.format=json",
		// Maximum time to wait for cluster connections to settle before evaluating notifications.
		fmt.Sprintf("--cluster.settle-timeout=%s", r.gw.Spec.Alerting.ClusterSettleTimeout),
		//Interval for gossip state syncs. Setting this interval lower (more frequent)
		// will increase convergence speeds across larger clusters at the expense
		// of increased bandwidth usage.
		fmt.Sprintf("--cluster.pushpull-interval=%s", r.gw.Spec.Alerting.ClusterPushPullInterval),
		// Interval between sending gossip messages. By lowering this value (more frequent)
		// gossip messages are propagated across the cluster more quickly at the expense of increased
		// bandwidth.
		fmt.Sprintf("--cluster.gossip-interval=%s", r.gw.Spec.Alerting.ClusterGossipInterval),
		// Time to wait between peers to send notifications
		"--cluster.peer-timeout=2s",
		fmt.Sprintf("--opni.listen-address=:%d", 3000),
	}
}

func (r *Reconciler) alertmanagerWorkerArgs() []string {
	return []string{
		fmt.Sprintf("--config.file=%s", r.configPath()),
		fmt.Sprintf("--storage.path=%s", r.gw.Spec.Alerting.DataMountPath),
		fmt.Sprintf("--log.level=%s", "info"),
		"--log.format=json",
		// join to controller
		fmt.Sprintf("--cluster.peer=%s:%d",
			shared.OperatorAlertingControllerServiceName,
			r.gw.Spec.Alerting.ClusterPort,
		),
		fmt.Sprintf("--opni.listen-address=:%d", 3000),
	}
}

func (r *Reconciler) syncerArgs() []string {
	_, gatewayPort, _ := net.SplitHostPort(strings.TrimPrefix(r.gw.Spec.Management.GetGRPCListenAddress(), "tcp://"))
	return []string{
		fmt.Sprintf("--syncer.alertmanager.config.file=%s", r.configPath()),
		fmt.Sprintf("--syncer.listen.address=:%d", 4000),
		fmt.Sprintf("--syncer.alertmanager.address=%s", fmt.Sprintf("http://localhost:%d", r.gw.Spec.Alerting.WebPort)),
		fmt.Sprintf("--syncer.gateway.join.address=opni-internal:%s", gatewayPort),
	}
}

func (r *Reconciler) configPath() string {
	return fmt.Sprintf("%s/%s", r.gw.Spec.Alerting.DataMountPath, shared.AlertManagerConfigKey)
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
				MountPath: r.gw.Spec.Alerting.DataMountPath,
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
	}
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
				MountPath: r.gw.Spec.Alerting.DataMountPath,
			},
		},
	}

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
			Type:     r.gw.Spec.Alerting.ServiceType,
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
			Replicas: &replicas,
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

func (r *Reconciler) alerting() []resources.Resource {
	// always create the alerting service, even if it points to nothing, so cortex can find it

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

	alertingMutator(&r.gw.Spec.Alerting)
	var resourceRequirements corev1.ResourceRequirements

	if value, err := resource.ParseQuantity(r.gw.Spec.Alerting.Storage); err == nil {
		resourceRequirements = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: value,
			},
		}
	}

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
				Resources: resourceRequirements,
			},
		},
	}

	_, _, natsVolumes := k8snats.ExternalNatsObjects(
		r.ctx,
		r.client,
		types.NamespacedName{
			Name:      r.gw.Spec.NatsRef.Name,
			Namespace: r.gw.Namespace,
		},
	)

	requiredVolumes = append(requiredVolumes, natsVolumes...)

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
		r.gw.Spec.Alerting.Replicas-1,
	)
	ctrl.SetControllerReference(r.gw, controllerService, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, controllerWorkers, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, workerService, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, workerWorkers, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, controllerService),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, controllerWorkers),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled && r.gw.Spec.Alerting.Replicas > 1, workerWorkers),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, workerService),
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
