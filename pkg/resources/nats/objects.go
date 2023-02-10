package nats

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	emperrors "emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/nats-io/nkeys"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	natsDefaultImage               = "nats:2.8.4-alpine"
	natsDefaultConfigReloaderImage = "natsio/nats-server-config-reloader:0.7.2"
	natsDefaultPidFilePath         = "/opt/nats/pid"
	natsDefaultPidFileName         = "nats-server.pid"
	natsDefaultConfigPath          = "/opt/nats/conf"
	natsConfigFileName             = "nats-config.conf"
	natsDefaultClientPort          = 4222
	natsDefaultClusterPort         = 6222
	natsDefaultHTTPPort            = 8222
)

var (
	natsConfigTemplate = template.Must(template.New("natsconfig").Parse(`
listen: 0.0.0.0:{{ .ClientPort }}
http: 0.0.0.0:{{ .HTTPPort }}
server_name: $POD_NAME


#Authorization for client connections
authorization {
	{{- if eq .AuthMethod "password" }}
	user: "{{ .Username }}"
	password: "{{ .Password }}"
	{{- else if eq .AuthMethod "nkey" }}
	users: [
		{ nkey: {{ .NKeyUser }} }
	]
	{{- end }}
	timeout 1
}

lame_duck_duration: "30s"

pid_file: "{{ .PidFile }}"

max_payload: 8388608

#Clustering defition
cluster {
	name: {{ .ClusterName }}
	listen: 0.0.0.0:{{ .ClusterPort }}

	authorization {
		user: "nats_cluster"
		password: "{{ .ClusterPassword }}"
		timeout: 1
	}

	routes = [
		nats://nats_cluster:{{ .ClusterPassword }}@{{ .ClusterURL }}:{{ .ClusterPort }}
	]
}
{{- if .Jetstream.Enabled }}
jetstream {
	{{- if not (eq .Jetstream.Mem "") }}
	max_mem: {{ .Jetstream.Mem }}
	{{- end }}
	{{- if .Jetstream.Storage.Enabled }}
	store_dir: /var/data
	max_file: {{ .Jetstream.Storage.Size }}
	{{- end }}

}
{{- end }}
	`))
)

type jetstreamFileConfigData struct {
	Enabled bool
	Size    string
}

type jestreamConfigData struct {
	Enabled bool
	Mem     string
	Storage jetstreamFileConfigData
}

type natsConfigData struct {
	AuthMethod      opnicorev1beta1.NatsAuthMethod
	Username        string
	Password        string
	NKeyUser        string
	ClientPort      int
	ClusterPort     int
	HTTPPort        int
	PidFile         string
	ClusterPassword string
	ClusterURL      string
	ClusterName     string
	Jetstream       jestreamConfigData
}

func (r *Reconciler) nats() (resourceList []resources.Resource, retErr error) {
	if lo.FromPtrOr(r.natsCluster.Spec.Replicas, 3) == 0 {
		return []resources.Resource{
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.natsStatefulSet(), reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-nats-config", r.natsCluster.Name),
						Namespace: r.natsCluster.Namespace,
						Labels:    r.natsLabels(),
					},
				}, reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.natsHeadlessService(), reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.natsClusterService(), reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.natsClientService(), reconciler.StateAbsent, nil
			},
		}, nil
	}

	config, err := r.natsConfig()
	if err != nil {
		retErr = emperrors.WithMessage(err, "failed to generate nats config")
		return
	}
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return config, reconciler.StatePresent, nil
	})

	if r.natsCluster.Spec.PasswordFrom != nil {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
				return err
			}
			r.natsCluster.Status.AuthSecretKeyRef = r.natsCluster.Spec.PasswordFrom
			return r.client.Status().Update(r.ctx, r.natsCluster)
		})
		if err != nil {
			retErr = err
			return
		}
	} else {
		secret, err := r.natsAuthSecret()
		if err != nil {
			retErr = emperrors.WithMessage(err, "failed to create auth secret")
			return
		}
		resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
			return secret, reconciler.StatePresent, nil
		})
	}

	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsHeadlessService(), reconciler.StatePresent, nil
	})
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsClusterService(), reconciler.StatePresent, nil
	})
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsClientService(), reconciler.StatePresent, nil
	})
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsStatefulSet(), reconciler.StatePresent, nil
	})

	return
}

func (r *Reconciler) natsStatefulSet() *appsv1.StatefulSet {
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: r.getReplicas(),
			Selector: &metav1.LabelSelector{
				MatchLabels: r.natsLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.natsLabels(),
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: lo.ToPtr[int64](40),
					ShareProcessNamespace:         lo.ToPtr(true),
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									PodAffinityTerm: corev1.PodAffinityTerm{
										TopologyKey: resources.HostTopologyKey,
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: r.natsLabels(),
										},
									},
									Weight: 100,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "nats",
							Image:           natsDefaultImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"nats-server",
							},
							Args: []string{
								"-c",
								fmt.Sprintf("%s/%s", natsDefaultConfigPath, natsConfigFileName),
							},
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: natsDefaultClientPort,
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: natsDefaultClusterPort,
									Name:          "cluster",
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: natsDefaultHTTPPort,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								FailureThreshold: 6,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/",
										Port:   intstr.FromString("http"),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
							},
							ReadinessProbe: &corev1.Probe{
								FailureThreshold: 6,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/",
										Port:   intstr.FromString("http"),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      5,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: natsDefaultConfigPath,
								},
								{
									Name:      "pid",
									MountPath: natsDefaultPidFilePath,
								},
							},
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"nats-server",
											"--signal",
											"ldm",
										},
									},
								},
							},
							Resources: func() corev1.ResourceRequirements {
								if r.natsCluster.Spec.JetStream.MemoryStorageSize.IsZero() {
									return corev1.ResourceRequirements{}
								}
								return corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceMemory: r.natsCluster.Spec.JetStream.MemoryStorageSize,
									},
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: func() resource.Quantity {
											memoryCopy := r.natsCluster.Spec.JetStream.MemoryStorageSize.DeepCopy()
											memoryCopy.Add(resource.MustParse("1Gi"))
											return memoryCopy
										}(),
									},
								}
							}(),
						},
						{
							Name:            "config-reloader",
							Image:           natsDefaultConfigReloaderImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"nats-server-config-reloader",
							},
							Args: []string{
								"-config",
								fmt.Sprintf("%s/%s", natsDefaultConfigPath, natsConfigFileName),
								"-pid",
								fmt.Sprintf("%s/%s", natsDefaultPidFilePath, natsDefaultPidFileName),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: natsDefaultConfigPath,
								},
								{
									Name:      "pid",
									MountPath: natsDefaultPidFilePath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  fmt.Sprintf("%s-nats-config", r.natsCluster.Name),
									DefaultMode: lo.ToPtr[int32](420),
								},
							},
						},
						{
							Name: "pid",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					NodeSelector: r.natsCluster.Spec.NodeSelector,
					Tolerations:  r.natsCluster.Spec.Tolerations,
				},
			},
		},
	}

	if lo.FromPtrOr(r.natsCluster.Spec.JetStream.Enabled, true) &&
		lo.FromPtrOr(r.natsCluster.Spec.JetStream.FileStorage.Enabled, true) {
		statefulset.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			statefulset.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "data",
				MountPath: "/var/data",
			},
		)
		volume, pvc := r.dataStorage()
		if volume != nil {
			statefulset.Spec.Template.Spec.Volumes = append(statefulset.Spec.Template.Spec.Volumes, *volume)
		}
		if pvc != nil {
			statefulset.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
				*pvc,
			}
		}
	}

	ctrl.SetControllerReference(r.natsCluster, statefulset, r.client.Scheme())
	return statefulset
}

func (r *Reconciler) natsConfig() (*corev1.Secret, error) {
	jetStreamFileConfig := jetstreamFileConfigData{
		Enabled: lo.FromPtrOr(r.natsCluster.Spec.JetStream.FileStorage.Enabled, true),
		Size:    r.natsCluster.Spec.JetStream.FileStorage.Size.String(),
	}

	jetStreamConfig := jestreamConfigData{
		Enabled: lo.FromPtrOr(r.natsCluster.Spec.JetStream.Enabled, true),
		Mem: func() string {
			if r.natsCluster.Spec.JetStream.MemoryStorageSize.IsZero() {
				return ""
			}
			return r.natsCluster.Spec.JetStream.MemoryStorageSize.String()
		}(),
		Storage: jetStreamFileConfig,
	}

	natsConfig := natsConfigData{
		ClusterName: r.natsCluster.Name,
		AuthMethod:  r.natsCluster.Spec.AuthMethod,
		ClientPort:  natsDefaultClientPort,
		HTTPPort:    natsDefaultHTTPPort,
		ClusterPort: natsDefaultClusterPort,
		PidFile:     fmt.Sprintf("%s/%s", natsDefaultPidFilePath, natsDefaultPidFileName),
		ClusterURL:  fmt.Sprintf("%s-nats-cluster", r.natsCluster.Name),
		Jetstream:   jetStreamConfig,
	}

	passwordBytes, err := r.getNatsClusterPassword()
	if err != nil {
		return &corev1.Secret{}, err
	}
	natsConfig.ClusterPassword = string(passwordBytes)

	switch r.natsCluster.Spec.AuthMethod {
	case opnicorev1beta1.NatsAuthPassword:
		if r.natsCluster.Spec.Username == "" {
			natsConfig.Username = "nats-user"
		} else {
			natsConfig.Username = r.natsCluster.Spec.Username
		}
		password, err := r.getNatsUserPassword()
		if err != nil {
			return &corev1.Secret{}, err
		}
		natsConfig.Password = string(password)
	case opnicorev1beta1.NatsAuthNkey:
		nKeyUser, _, err := r.getNKeyUser()
		if err != nil {
			return &corev1.Secret{}, err
		}
		natsConfig.NKeyUser = nKeyUser
	default:
		return &corev1.Secret{}, errors.New("nats auth method not supported")
	}

	var buffer bytes.Buffer
	natsConfigTemplate.Execute(&buffer, natsConfig)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-config", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Data: map[string][]byte{
			natsConfigFileName: buffer.Bytes(),
		},
	}
	err = ctrl.SetControllerReference(r.natsCluster, secret, r.client.Scheme())
	return secret, err
}

func (r *Reconciler) getNatsClusterPassword() ([]byte, error) {
	return r.fetchOrGeneratePassword("cluster-password")
}

func (r *Reconciler) natsAuthSecret() (*corev1.Secret, error) {
	switch r.natsCluster.Spec.AuthMethod {
	case opnicorev1beta1.NatsAuthPassword:
		password, err := r.getNatsUserPassword()
		if err != nil {
			return nil, err
		}
		secret := r.genericAuthSecret("password", password)
		err = ctrl.SetControllerReference(r.natsCluster, secret, r.client.Scheme())
		if err != nil {
			return nil, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
				return err
			}
			r.natsCluster.Status.AuthSecretKeyRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: "password",
			}
			return r.client.Status().Update(r.ctx, r.natsCluster)
		})
		if err != nil {
			return nil, err
		}
		return secret, nil
	case opnicorev1beta1.NatsAuthNkey:
		_, seed, err := r.getNKeyUser()
		if err != nil {
			return nil, err
		}
		secret := r.genericAuthSecret("seed", seed)
		err = ctrl.SetControllerReference(r.natsCluster, secret, r.client.Scheme())
		if err != nil {
			return nil, err
		}

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
				return err
			}
			r.natsCluster.Status.AuthSecretKeyRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: "seed",
			}
			return r.client.Status().Update(r.ctx, r.natsCluster)
		})
		if err != nil {
			return nil, err
		}

		return secret, nil
	default:
		return nil, errors.New("nats auth method not supported")
	}
}

func (r *Reconciler) natsHeadlessService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-headless", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-client",
					Port:       natsDefaultClientPort,
					TargetPort: intstr.FromString("client"),
				},
				{
					Name:       "tcp-cluster",
					Port:       natsDefaultClusterPort,
					TargetPort: intstr.FromString("cluster"),
				},
			},
			Selector: r.natsLabels(),
		},
	}
	ctrl.SetControllerReference(r.natsCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) natsClusterService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-cluster", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-cluster",
					Port:       natsDefaultClusterPort,
					TargetPort: intstr.FromString("cluster"),
				},
			},
			Selector: r.natsLabels(),
		},
	}
	ctrl.SetControllerReference(r.natsCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) natsClientService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-client", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-client",
					Port:       natsDefaultClientPort,
					TargetPort: intstr.FromString("client"),
				},
			},
			Selector: r.natsLabels(),
		},
	}
	ctrl.SetControllerReference(r.natsCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) getReplicas() *int32 {
	if r.natsCluster.Spec.Replicas == nil {
		return lo.ToPtr[int32](3)
	}
	return r.natsCluster.Spec.Replicas
}

func (r *Reconciler) genericAuthSecret(key string, data []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-client", r.natsCluster.Name),
			Namespace: r.natsCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Data: map[string][]byte{
			key: data,
		},
	}
}

// fetchOrGeneratePassword will check if there is already a nats password stored in the cluster state secret
// and return the value.  If there is no secret it will generate a new password, store it in the secret,
// and then return the generated value
func (r *Reconciler) fetchOrGeneratePassword(key string) ([]byte, error) {
	secret := corev1.Secret{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats-secrets", r.natsCluster.Name),
		Namespace: r.natsCluster.Namespace,
	}, &secret)
	if k8serrors.IsNotFound(err) {
		password := util.GenerateRandomString(8)

		err := r.updateState(key, password)
		if err != nil {
			return make([]byte, 0), err
		}

		return password, nil
	} else if err != nil {
		return make([]byte, 0), err
	}
	password, ok := secret.Data[key]
	if !ok {
		password = util.GenerateRandomString(8)
		err := r.updateState(key, password)
		if err != nil {
			return make([]byte, 0), err
		}
	}
	return password, nil
}

func (r *Reconciler) updateState(key string, value []byte) error {
	secret := &corev1.Secret{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats-secrets", r.natsCluster.Name),
		Namespace: r.natsCluster.Namespace,
	}, secret)
	if k8serrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-secrets", r.natsCluster.Name),
				Namespace: r.natsCluster.Namespace,
			},
			Data: map[string][]byte{
				key: value,
			},
		}
		err = ctrl.SetControllerReference(r.natsCluster, secret, r.client.Scheme())
		if err != nil {
			return err
		}
		return r.client.Create(r.ctx, secret)
	} else if err != nil {
		return err
	}
	secret.Data[key] = value
	return r.client.Update(r.ctx, secret)
}

// getNatsUserPassword gets the password provided by PasswordFrom in the nats Spec.
// If that doesn't exist it will check if a password has previously been generated
// and return that.  Otherwise it will generate a new password.
func (r *Reconciler) getNatsUserPassword() ([]byte, error) {
	if r.natsCluster.Spec.PasswordFrom != nil {
		secret := corev1.Secret{}
		if err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.natsCluster.Spec.PasswordFrom.Name,
			Namespace: r.natsCluster.Namespace,
		}, &secret); err != nil {
			return make([]byte, 0), err
		}
		password, ok := secret.Data[r.natsCluster.Spec.PasswordFrom.Key]
		if !ok {
			return make([]byte, 0), errors.New("key does not exist in secret")
		}
		return password, nil
	}
	return r.fetchOrGeneratePassword("user-password")
}

// getNKeyUser will check if there is already a nkey seed stored in a secret
// it will check if there is a publickey, and return it or generate a new public key.
// If there is no seed it will generate a new keypair, store the seed, and return the
// public key
func (r *Reconciler) getNKeyUser() (string, []byte, error) {
	var seed []byte
	var err error
	var publicKey string
	secret := corev1.Secret{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats-secrets", r.natsCluster.Name),
		Namespace: r.natsCluster.Namespace,
	}, &secret)
	if k8serrors.IsNotFound(err) {
		user, err := nkeys.CreateUser()
		if err != nil {
			return "", make([]byte, 0), err
		}
		seed, err = user.Seed()
		if err != nil {
			return "", make([]byte, 0), err
		}
		publicKey, err = user.PublicKey()
		if err != nil {
			return "", make([]byte, 0), err
		}

		err = r.updateState("seed", seed)
		if err != nil {
			return "", make([]byte, 0), err
		}
	} else if err != nil {
		return "", make([]byte, 0), err
	}

	seed, ok := secret.Data["seed"]
	if !ok {
		user, err := nkeys.CreateUser()
		if err != nil {
			return "", make([]byte, 0), err
		}
		seed, err = user.Seed()
		if err != nil {
			return "", make([]byte, 0), err
		}
		publicKey, err = user.PublicKey()
		if err != nil {
			return "", make([]byte, 0), err
		}

		err = r.updateState("seed", seed)
		if err != nil {
			return "", make([]byte, 0), err
		}
	}

	if publicKey != "" {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
				return err
			}
			r.natsCluster.Status.NKeyUser = publicKey
			return r.client.Status().Update(r.ctx, r.natsCluster)
		})
		if err != nil {
			return "", make([]byte, 0), err
		}
		return publicKey, seed, nil
	}

	if r.natsCluster.Status.NKeyUser == "" {
		user, err := nkeys.FromSeed(seed)
		if err != nil {
			return "", make([]byte, 0), err
		}
		publicKey, err = user.PublicKey()
		if err == nil {
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
					return err
				}
				r.natsCluster.Status.NKeyUser = publicKey
				return r.client.Status().Update(r.ctx, r.natsCluster)
			})
			if err != nil {
				return "", make([]byte, 0), err
			}
		}
		return publicKey, seed, err
	}

	return r.natsCluster.Status.NKeyUser, seed, nil
}

func (r *Reconciler) natsLabels() map[string]string {
	return map[string]string{
		resources.AppNameLabel:    "nats",
		resources.OpniClusterName: r.natsCluster.Name,
	}
}

func (r *Reconciler) dataStorage() (*corev1.Volume, *corev1.PersistentVolumeClaim) {
	if r.natsCluster.Spec.JetStream.FileStorage.JetStreamPersistenceSpec.EmptyDir != nil {
		return &corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: r.natsCluster.Spec.JetStream.FileStorage.JetStreamPersistenceSpec.EmptyDir,
			},
		}, nil
	}
	if r.natsCluster.Spec.JetStream.FileStorage.JetStreamPersistenceSpec.PVC != nil {
		pvc := r.natsCluster.Spec.JetStream.FileStorage.JetStreamPersistenceSpec.PVC
		return nil, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: pvc.AccessModes,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: r.natsCluster.Spec.JetStream.FileStorage.Size,
					},
				},
				StorageClassName: &pvc.StorageClassName,
				VolumeMode:       lo.ToPtr(corev1.PersistentVolumeFilesystem),
			},
		}
	}

	return nil, nil
}
