package gateway

import (
	"bytes"
	"fmt"
	"path"

	"github.com/rancher/opni/apis/v1beta2"
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

func setEmptyFields(gw *v1beta2.Gateway) {
	// handle missing fields because the test suite is flaky locally
	if gw.Spec.Alerting.WebPort == 0 {
		gw.Spec.Alerting.WebPort = 9093
	}

	if gw.Spec.Alerting.ClusterPort == 0 {
		gw.Spec.Alerting.ClusterPort = 9094
	}

	if gw.Spec.Alerting.Storage == "" {
		gw.Spec.Alerting.Storage = "500Mi"
	}
	if gw.Spec.Alerting.ConfigName == "" {
		gw.Spec.Alerting.ConfigName = "alertmanager-config"
	}
	if gw.Spec.Alerting.RawConfigMap == "" {
		var amData bytes.Buffer
		mgmtDNS := "opni-monitoring-internal"
		httpPort := "11080"
		amData, err := shared.DefaultConfig(fmt.Sprintf("http://%s:%s%s", mgmtDNS, httpPort, shared.AlertingCortexHookHandler))
		if err != nil {
			panic(err)
		}
		gw.Spec.Alerting.RawConfigMap = amData.String()
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

	if r.gw.Spec.Alerting == nil {
		// set some sensible defaults
		r.spec.Alerting = &corev1beta1.AlertingSpec{
			WebPort:     9093,
			ClusterPort: 9094,
			Storage:     "500Mi",
			ServiceType: "ClusterIP",
			ConfigName:  "alertmanager-config",
		}
	}
	setEmptyFields(r.gw)
	dataMountPath := shared.DataMountPath
	configMountPath := shared.ConfigMountPath

	// to be mounted into the alertmanager controller node
	alertManagerConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.gw.Spec.Alerting.ConfigName,
			Namespace: r.gw.Namespace,
		},

		Data: map[string]string{
			shared.ConfigKey: r.gw.Spec.Alerting.RawConfigMap,
		},
	}
	ctrl.SetControllerReference(r.gw, alertManagerConfigMap, r.client.Scheme())

	alertingControllerSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName,
			Namespace: r.gw.Namespace,
			Labels:    publicControllerSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.gw.Spec.Alerting.ServiceType,
			Selector: publicControllerLabels,
			Ports:    r.serviceAlertManagerPorts(r.controllerAlertManagerPorts()),
		},
	}

	ctrl.SetControllerReference(r.gw, alertingControllerSvc, r.client.Scheme())

	controllerDeploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingControllerServiceName + "-internal",
			Namespace: r.gw.Namespace,
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
							},
							Name:            "opni-alertmanager",
							Image:           "bitnami/alertmanager:latest",
							ImagePullPolicy: "Always",
							// Defaults to
							// "--config.file=/opt/bitnami/alertmanager/conf/config.yml",
							// "--storage.path=/opt/bitnami/alertmanager/data"
							Args: []string{
								fmt.Sprintf("--cluster.listen-address=0.0.0.0:%d", r.gw.Spec.Alerting.ClusterPort),
								fmt.Sprintf("--config.file=%s", path.Join(configMountPath, shared.ConfigKey)),
								fmt.Sprintf("--storage.path=%s", dataMountPath),
								fmt.Sprintf("--log.level=%s", "debug"),
								fmt.Sprintf("--log.format=json"),
								// Maximum time to wait for cluster connections to settle before evaluating notifications.
								fmt.Sprintf("--cluster.settle-timeout=%s", "10s"),
								//Interval for gossip state syncs. Setting this interval lower (more frequent)
								// will increase convergence speeds across larger clusters at the expense
								// of increased bandwidth usage.
								fmt.Sprintf("--cluster.pushpull-interval=%s", "20s"),
								// Interval between sending gossip messages. By lowering this value (more frequent)
								// gossip messages are propagated across the cluster more quickly at the expense of increased
								// bandwidth.
								fmt.Sprintf("--cluster.gossip-interval=%s", "20ms"),
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
										Name: r.gw.Spec.Alerting.ConfigName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  shared.ConfigKey,
											Path: shared.ConfigKey,
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
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(r.gw.Spec.Alerting.Storage),
							},
						},
					}},
			},
		},
	}

	ctrl.SetControllerReference(r.gw, controllerDeploy, r.client.Scheme())

	alertingClusterNodeSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        shared.OperatorAlertingClusterNodeServiceName,
			Namespace:   r.gw.Namespace,
			Labels:      publicNodeSvcLabels,
			Annotations: r.gw.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.gw.Spec.Alerting.ServiceType,
			Selector: publicNodeLabels,
			Ports:    r.serviceAlertManagerPorts(r.nodeAlertManagerPorts()),
		},
	}
	ctrl.SetControllerReference(r.gw, alertingClusterNodeSvc, r.client.Scheme())

	nodeDeploy := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shared.OperatorAlertingClusterNodeServiceName + "internal",
			Namespace: r.gw.Namespace,
			Labels:    publicNodeLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr(r.numAlertingReplicas()),
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
							Image:           "bitnami/alertmanager:latest",
							ImagePullPolicy: "Always",
							// Defaults to
							// "--config.file=/opt/bitnami/alertmanager/conf/config.yml",
							// "--storage.path=/opt/bitnami/alertmanager/data"
							Args: []string{
								fmt.Sprintf("--config.file=%s", path.Join(configMountPath, shared.ConfigKey)),
								fmt.Sprintf("--storage.path=%s", dataMountPath),
								fmt.Sprintf("--log.level=%s", "debug"),
								fmt.Sprintf("--log.format=json"),
								// join to controller
								fmt.Sprintf("--cluster.peer=%s:%d",
									shared.OperatorAlertingControllerServiceName,
									r.gw.Spec.Alerting.ClusterPort,
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
											Key:  shared.ConfigKey,
											Path: shared.ConfigKey,
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
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(r.spec.Alerting.Storage),
							},
						},
					}},
			},
		},
	}
	ctrl.SetControllerReference(r.gw, nodeDeploy, r.client.Scheme())

	return []resources.Resource{
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, controllerDeploy),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, alertingControllerSvc),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, alertManagerConfigMap),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, nodeDeploy),
		resources.PresentIff(r.gw.Spec.Alerting.Enabled, alertingClusterNodeSvc),
	}
}

func (r *Reconciler) numAlertingReplicas() int32 {
	return 3
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
