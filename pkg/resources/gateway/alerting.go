package gateway

import (
	"bytes"
	"fmt"
	"path"

	"github.com/rancher/opni/pkg/alerting/routing"
	"gopkg.in/yaml.v3"

	"github.com/rancher/opni/apis/core/v1beta1"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// dynamic config
	if spec.ConfigName == "" {
		spec.ConfigName = "alertmanager-config"
	}
	if spec.RawAlertManagerConfig == "" {
		var amData bytes.Buffer
		mgmtDNS := "opni-internal"
		httpPort := "11080"
		amData, err := shared.DefaultAlertManagerConfig(fmt.Sprintf("http://%s:%s%s", mgmtDNS, httpPort, shared.AlertingCortexHookHandler))
		if err != nil {
			panic(err)
		}
		spec.RawAlertManagerConfig = amData.String()
	}
	if spec.RawInternalRouting == "" {
		ir := routing.NewDefaultOpniInternalRouting()
		rtData, err := yaml.Marshal(ir)
		if err != nil {
			panic(err)
		}
		spec.RawInternalRouting = string(rtData)
	}
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

	alertingMutator(&r.spec.Alerting)
	dataMountPath := shared.DataMountPath
	configMountPath := shared.ConfigMountPath

	// to be mounted into the alertmanager controller node
	alertManagerConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.spec.Alerting.ConfigName,
			Namespace: r.namespace,
		},

		Data: map[string]string{
			shared.AlertManagerConfigKey:    r.spec.Alerting.RawAlertManagerConfig,
			shared.InternalRoutingConfigKey: r.spec.Alerting.RawInternalRouting,
		},
	}

	alertingControllerSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName,
			Namespace: r.namespace,
			Labels:    publicControllerSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.spec.Alerting.ServiceType,
			Selector: publicControllerLabels,
			Ports:    r.serviceAlertManagerPorts(r.controllerAlertManagerPorts()),
		},
	}

	var resourceRequirements corev1.ResourceRequirements

	if value, err := resource.ParseQuantity(r.spec.Alerting.Storage); err == nil {
		resourceRequirements = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: value,
			},
		}
	}

	controllerDeploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName + "-internal",
			Namespace: r.namespace,
			Labels:    publicControllerLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: publicControllerLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: publicControllerLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  lo.ToPtr(int64(1099)),
						RunAsGroup: lo.ToPtr(int64(1099)),
						FSGroup:    lo.ToPtr(int64(1099)),
					},
					Containers: []corev1.Container{
						{
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
							Image:           r.statusImage(),
							ImagePullPolicy: "Always",
							// Defaults to
							// "--config.file=/opt/bitnami/alertmanager/conf/config.yml",
							// "--storage.path=/opt/bitnami/alertmanager/data"
							Args: []string{
								"alertmanager",
								fmt.Sprintf("--cluster.listen-address=0.0.0.0:%d", r.spec.Alerting.ClusterPort),
								fmt.Sprintf("--config.file=%s", path.Join(configMountPath, shared.AlertManagerConfigKey)),
								fmt.Sprintf("--storage.path=%s", dataMountPath),
								fmt.Sprintf("--log.level=%s", "debug"),
								fmt.Sprintf("--log.format=json"),
								// Maximum time to wait for cluster connections to settle before evaluating notifications.
								fmt.Sprintf("--cluster.settle-timeout=%s", r.spec.Alerting.ClusterSettleTimeout),
								//Interval for gossip state syncs. Setting this interval lower (more frequent)
								// will increase convergence speeds across larger clusters at the expense
								// of increased bandwidth usage.
								fmt.Sprintf("--cluster.pushpull-interval=%s", r.spec.Alerting.ClusterPushPullInterval),
								// Interval between sending gossip messages. By lowering this value (more frequent)
								// gossip messages are propagated across the cluster more quickly at the expense of increased
								// bandwidth.
								fmt.Sprintf("--cluster.gossip-interval=%s", r.spec.Alerting.ClusterGossipInterval),
								// Time to wait between peers to send notifications
								fmt.Sprintf("--cluster.peer-timeout=1s"),
							},
							Ports: r.controllerAlertManagerPorts(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "opni-alertmanager-data",
									MountPath: dataMountPath,
								},
								// volume mount for alertmanager configmap
								{
									Name:      "opni-alertmanager-config",
									MountPath: configMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "opni-alertmanager-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "opni-alertmanager-data",
									ReadOnly:  false,
								},
							},
						},
						// mount alertmanager config map to volume
						{
							Name: "opni-alertmanager-config",
							VolumeSource: corev1.VolumeSource{
								//PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								//	ClaimName: "opni-alertmanager-data",
								//},
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: r.spec.Alerting.ConfigName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  shared.AlertManagerConfigKey,
											Path: shared.AlertManagerConfigKey,
										},
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
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
			},
		},
	}
	alertingClusterNodeSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        shared.OperatorAlertingClusterNodeServiceName,
			Namespace:   r.namespace,
			Labels:      publicNodeSvcLabels,
			Annotations: r.spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.spec.Alerting.ServiceType,
			Selector: publicNodeLabels,
			Ports:    r.serviceAlertManagerPorts(r.nodeAlertManagerPorts()),
		},
	}
	nodeDeploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingClusterNodeServiceName + "-internal",
			Namespace: r.namespace,
			Labels:    publicNodeLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr(r.spec.Alerting.Replicas - 1),
			Selector: &metav1.LabelSelector{
				MatchLabels: publicNodeLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: publicNodeLabels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  lo.ToPtr(int64(1099)),
						RunAsGroup: lo.ToPtr(int64(1099)),
						FSGroup:    lo.ToPtr(int64(1099)),
					},
					Containers: []corev1.Container{

						{
							Name:            "opni-alertmanager",
							Image:           r.statusImage(),
							ImagePullPolicy: "Always",
							Env: []corev1.EnvVar{
								{
									Name:  "USER",
									Value: "alerting",
								},
							},
							// Defaults to
							// "--config.file=/opt/bitnami/alertmanager/conf/config.yml",
							// "--storage.path=/opt/bitnami/alertmanager/data"
							Args: []string{
								"alertmanager",
								fmt.Sprintf("--config.file=%s", path.Join(configMountPath, shared.AlertManagerConfigKey)),
								fmt.Sprintf("--storage.path=%s", dataMountPath),
								fmt.Sprintf("--log.level=%s", "info"),
								fmt.Sprintf("--log.format=json"),
								// join to controller
								fmt.Sprintf("--cluster.peer=%s:%d",
									shared.OperatorAlertingControllerServiceName,
									r.spec.Alerting.ClusterPort,
								),
								//fmt.Sprintf(
								//	"--cluster.advertise-address=%s:%d",
								//	shared.OperatorAlertingControllerServiceName,
								//	r.gw.Spec.Alerting.ClusterPort),
							},
							Ports: r.nodeAlertManagerPorts(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "opni-alertmanager-data",
									MountPath: dataMountPath,
								},
								// volume mount for alertmanager configmap
								{
									Name:      "opni-alertmanager-config",
									MountPath: configMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "opni-alertmanager-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "opni-alertmanager-data",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: "opni-alertmanager-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: r.spec.Alerting.ConfigName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  shared.AlertManagerConfigKey,
											Path: shared.AlertManagerConfigKey,
										},
										{
											Key:  shared.InternalRoutingConfigKey,
											Path: shared.InternalRoutingConfigKey,
										},
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
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
			},
		},
	}
	if r.gw != nil {
		ctrl.SetControllerReference(r.gw, nodeDeploy, r.client.Scheme())
		ctrl.SetControllerReference(r.gw, alertingClusterNodeSvc, r.client.Scheme())
		ctrl.SetControllerReference(r.gw, controllerDeploy, r.client.Scheme())
		ctrl.SetControllerReference(r.gw, alertingControllerSvc, r.client.Scheme())
		ctrl.SetControllerReference(r.gw, alertManagerConfigMap, r.client.Scheme())
		return []resources.Resource{
			resources.PresentIff(r.spec.Alerting.Enabled, controllerDeploy),
			resources.PresentIff(r.spec.Alerting.Enabled, alertingControllerSvc),
			resources.PresentIff(r.spec.Alerting.Enabled, alertManagerConfigMap),
			resources.PresentIff(r.spec.Alerting.Enabled && r.spec.Alerting.Replicas > 1, nodeDeploy),
			resources.PresentIff(r.spec.Alerting.Enabled, alertingClusterNodeSvc),
		}
	}
	ctrl.SetControllerReference(r.coreGW, nodeDeploy, r.client.Scheme())
	ctrl.SetControllerReference(r.coreGW, alertingClusterNodeSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.coreGW, controllerDeploy, r.client.Scheme())
	ctrl.SetControllerReference(r.coreGW, alertingControllerSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.coreGW, alertManagerConfigMap, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.spec.Alerting.Enabled, controllerDeploy),
		resources.PresentIff(r.spec.Alerting.Enabled, alertingControllerSvc),
		resources.PresentIff(r.spec.Alerting.Enabled, alertManagerConfigMap),
		resources.PresentIff(r.spec.Alerting.Enabled && r.spec.Alerting.Replicas > 1, nodeDeploy),
		resources.PresentIff(r.spec.Alerting.Enabled, alertingClusterNodeSvc),
	}

}

func (r *Reconciler) nodeAlertManagerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "web-port",
			ContainerPort: 9093,
			Protocol:      "TCP",
		},
	}
}

func (r *Reconciler) controllerAlertManagerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
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
