package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var etcdLabels = map[string]string{
	"app.kubernetes.io/name":    "etcd",
	"app.kubernetes.io/part-of": "opni",
}

func (r *Reconciler) etcd() ([]resources.Resource, error) {
	statefulset, err := r.etcdStatefulSet()
	if err != nil {
		return nil, err
	}
	secrets, err := r.etcdSecrets()
	if err != nil {
		return nil, err
	}
	services, err := r.etcdServices()
	if err != nil {
		return nil, err
	}

	resources := append([]resources.Resource{statefulset}, secrets...)
	resources = append(resources, services...)
	return resources, nil
}

func (r *Reconciler) etcdStatefulSet() (resources.Resource, error) {
	healthCheckProbe := corev1.ProbeHandler{
		Exec: &corev1.ExecAction{
			Command: []string{"/opt/bitnami/scripts/etcd/healthcheck.sh"},
		},
	}
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: r.namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: lo.ToPtr[int32](1),
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: etcdLabels,
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			ServiceName:         "etcd-headless",
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: etcdLabels,
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 1,
									PodAffinityTerm: corev1.PodAffinityTerm{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: etcdLabels,
										},
										Namespaces: []string{r.namespace},
									},
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: lo.ToPtr[int64](1001),
					},
					Containers: []corev1.Container{
						{
							Name:            "etcd",
							Image:           "docker.io/bitnami/etcd:3",
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: lo.ToPtr(true),
								RunAsUser:    lo.ToPtr[int64](1001),
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "client",
									ContainerPort: 2379,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "peer",
									ContainerPort: 2380,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "MY_POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name: "MY_POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name:  "MY_STS_NAME",
									Value: "etcd",
								},
								{
									Name:  "ETCDCTL_API",
									Value: "3",
								},
								{
									Name:  "ETCD_ON_K8S",
									Value: "yes",
								},
								{
									Name:  "ETCD_START_FROM_SNAPSHOT",
									Value: "no",
								},
								{
									Name:  "ETCD_DISASTER_RECOVERY",
									Value: "no",
								},
								{
									Name:  "ETCD_NAME",
									Value: "$(MY_POD_NAME)",
								},
								{
									Name:  "ETCD_DATA_DIR",
									Value: "/bitnami/etcd/data",
								},
								{
									Name:  "ETCD_LOG_LEVEL",
									Value: "info",
								},
								{
									Name:  "ALLOW_NONE_AUTHENTICATION",
									Value: "no",
								},
								{
									Name: "ETCD_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "etcd",
											},
											Key: "etcd-root-password",
										},
									},
								},
								{
									Name:  "ETCD_AUTH_TOKEN",
									Value: "jwt,priv-key=/opt/bitnami/etcd/certs/token/jwt-token.pem,sign-method=RS256,ttl=10m",
								},
								{
									Name:  "ETCD_ADVERTISE_CLIENT_URLS",
									Value: fmt.Sprintf("https://$(MY_POD_NAME).etcd-headless.%[1]s.svc.cluster.local:2379,https://etcd.%[1]s.svc.cluster.local:2379", r.namespace),
								},
								{
									Name:  "ETCD_LISTEN_CLIENT_URLS",
									Value: "https://0.0.0.0:2379",
								},
								{
									Name:  "ETCD_INITIAL_ADVERTISE_PEER_URLS",
									Value: fmt.Sprintf("http://$(MY_POD_NAME).etcd-headless.%s.svc.cluster.local:2380", r.namespace),
								},
								{
									Name:  "ETCD_LISTEN_PEER_URLS",
									Value: "http://0.0.0.0:2380",
								},
								{
									Name:  "ETCD_CLUSTER_DOMAIN",
									Value: fmt.Sprintf("etcd-headless.%s.svc.cluster.local", r.namespace),
								},
								{
									Name:  "ETCD_CERT_FILE",
									Value: "/opt/bitnami/etcd/certs/client/tls.crt",
								},
								{
									Name:  "ETCD_KEY_FILE",
									Value: "/opt/bitnami/etcd/certs/client/tls.key",
								},
								{
									Name:  "ETCD_CLIENT_CERT_AUTH",
									Value: "true",
								},
								{
									Name:  "ETCD_TRUSTED_CA_FILE",
									Value: "/opt/bitnami/etcd/certs/client/ca.crt",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler:        healthCheckProbe,
								InitialDelaySeconds: 5,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler:        healthCheckProbe,
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/bitnami/etcd",
								},
								{
									Name:      "etcd-jwt-token",
									MountPath: "/opt/bitnami/etcd/certs/token/",
									ReadOnly:  true,
								},
								{
									Name:      "etcd-client-certs",
									MountPath: "/opt/bitnami/etcd/certs/client/",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-jwt-token",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "etcd-jwt-token",
									DefaultMode: lo.ToPtr[int32](0400),
								},
							},
						},
						{
							Name: "etcd-client-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "etcd-serving-cert-keys",
									DefaultMode: lo.ToPtr[int32](0400),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("20Gi"),
							},
						},
					},
				},
			},
		},
	}

	r.setOwner(statefulset)
	return resources.Present(statefulset), nil
}

func (r *Reconciler) etcdSecrets() ([]resources.Resource, error) {
	password := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: r.namespace,
			Labels:    etcdLabels,
		},
	}
	err := r.client.Get(context.Background(), client.ObjectKeyFromObject(password), password)
	if err != nil && k8serrors.IsNotFound(err) {
		password.Type = corev1.SecretTypeOpaque
		password.StringData = map[string]string{
			"etcd-root-password": string(util.GenerateRandomString(32)),
		}
	}

	token := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-jwt-token",
			Namespace: r.namespace,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
	}
	err = r.client.Get(context.Background(), client.ObjectKeyFromObject(token), token)
	if err != nil && k8serrors.IsNotFound(err) {
		token.Type = corev1.SecretTypeOpaque
		privateKey := util.Must(rsa.GenerateKey(rand.Reader, 2048))
		data := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})
		token.Data = map[string][]byte{
			"jwt-token.pem": data,
		}
	}

	r.setOwner(password)
	return []resources.Resource{
		resources.Present(password),
		resources.Present(token),
	}, nil
}

func (r *Reconciler) etcdServices() ([]resources.Resource, error) {
	ports := []corev1.ServicePort{
		{
			Name:       "client",
			Port:       2379,
			TargetPort: intstr.FromString("client"),
		},
		{
			Name:       "peer",
			Port:       2380,
			TargetPort: intstr.FromString("peer"),
		},
	}
	headless := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-headless",
			Namespace: r.namespace,
			Labels:    etcdLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Ports:                    ports,
			Selector:                 etcdLabels,
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd",
			Namespace: r.namespace,
			Labels:    etcdLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports:           ports,
			Selector:        etcdLabels,
		},
	}

	r.setOwner(headless)
	r.setOwner(svc)
	return []resources.Resource{
		resources.Present(headless),
		resources.Present(svc),
	}, nil
}
