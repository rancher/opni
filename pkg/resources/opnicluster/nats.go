package opnicluster

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"text/template"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/go-logr/logr"
	"github.com/nats-io/nkeys"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"k8s.io/utils/pointer"
)

const (
	natsDefaultImage               = "bitnami/nats:2.3.2-debian-10-r0"
	natsDefaultConfigReloaderImage = "connecteverything/nats-server-config-reloader:0.4.5-v1alpha2"
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


#Authorization for client connections
authorization {
	{{- if eq .Nats.AuthMethod "username" }}
	user: "{{ .Nats.Username }}"
	password: "{{ .Nats.Password }}"
	{{- else if eq .Nats.AuthMethod "nkey" }}
	users: [
		{ nkey: {{ .NKeyUser }} }
	]
	{{- end }}
	timeout 1
}

lame_duck_duration: "30s"

pid_file: "{{ .PidFile }}"

#Clustering defition
cluster {
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
	`))
)

type natsConfigData struct {
	Nats            *v1beta1.NatsSpec
	NKeyUser        string
	ClientPort      int
	ClusterPort     int
	HTTPPort        int
	PidFile         string
	ClusterPassword string
	ClusterURL      string
}

func (r *Reconciler) nats() (resourceList []resources.Resource) {
	resourceList = []resources.Resource{}
	lg := logr.FromContext(r.ctx)

	if *r.getReplicas() == 0 {
		return []resources.Resource{
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.natsStatefulSet(), reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return r.nkeySecret([]byte("empty")), reconciler.StateAbsent, nil
			},
			func() (runtime.Object, reconciler.DesiredState, error) {
				return &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-nats-config", r.opniCluster.Name),
						Namespace: r.opniCluster.Namespace,
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
		}
	}

	config, err := r.natsConfig()
	if err != nil {
		lg.Error(err, "failed to generate config")
	}
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return config, reconciler.StatePresent, nil
	})

	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsHeadlessService(), reconciler.StatePresent, nil
	})
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsClusterService(), reconciler.StatePresent, nil
	})
	resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
		return r.natsClientService(), reconciler.StatePresent, nil
	})

	statefulset := appsv1.StatefulSet{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats", r.opniCluster.Name),
		Namespace: r.opniCluster.Namespace,
	}, &statefulset)
	if k8serrors.IsNotFound(err) {
		resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
			return r.natsStatefulSet(), reconciler.StatePresent, nil
		})
	} else if err != nil {
		lg.Error(err, "failed to get nats statefulset")
		return
	}

	if r.opniCluster.Status.NatsReplicas != 0 && *r.getReplicas() != r.opniCluster.Status.NatsReplicas {
		resourceList = append(resourceList, func() (runtime.Object, reconciler.DesiredState, error) {
			return r.natsStatefulSet(), reconciler.StatePresent, nil
		})
	}

	return resourceList
}

func (r *Reconciler) natsStatefulSet() *appsv1.StatefulSet {
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
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
					TerminationGracePeriodSeconds: pointer.Int64(40),
					ShareProcessNamespace:         pointer.Bool(true),
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
								Handler: corev1.Handler{
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
								Handler: corev1.Handler{
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
								PreStop: &corev1.Handler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"nats-server",
											"--signal",
											"ldm",
										},
									},
								},
							},
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
									SecretName:  fmt.Sprintf("%s-nats-config", r.opniCluster.Name),
									DefaultMode: pointer.Int32(420),
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
				},
			},
		},
	}
	ctrl.SetControllerReference(r.opniCluster, statefulset, r.client.Scheme())
	return statefulset
}

func (r *Reconciler) natsConfig() (*corev1.Secret, error) {
	natsConfig := natsConfigData{
		Nats:        &r.opniCluster.Spec.Nats,
		ClientPort:  natsDefaultClientPort,
		HTTPPort:    natsDefaultHTTPPort,
		ClusterPort: natsDefaultClusterPort,
		PidFile:     fmt.Sprintf("%s/%s", natsDefaultPidFilePath, natsDefaultPidFileName),
		ClusterURL:  fmt.Sprintf("%s-nats-cluster", r.opniCluster.Name),
	}

	switch r.opniCluster.Spec.Nats.AuthMethod {
	case v1beta1.NatsAuthUsername:
		passwordBytes, err := r.getNatsClusterPassword()
		if err != nil {
			return &corev1.Secret{}, err
		}
		natsConfig.ClusterPassword = string(passwordBytes)
	case v1beta1.NatsAuthNkey:
		nKeyUser, err := r.getNKeyUser()
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
			Name:      fmt.Sprintf("%s-nats-config", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Data: map[string][]byte{
			natsConfigFileName: buffer.Bytes(),
		},
	}
	ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
	return secret, nil
}

func (r *Reconciler) nkeySecret(nKeySeed []byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-nkey", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
			Labels:    r.natsLabels(),
		},
		Data: map[string][]byte{
			"seed": nKeySeed,
		},
	}
	ctrl.SetControllerReference(r.opniCluster, secret, r.client.Scheme())
	return secret
}

func (r *Reconciler) natsHeadlessService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-headless", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) natsClusterService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-cluster", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) natsClientService() *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-client", r.opniCluster.Name),
			Namespace: r.opniCluster.Namespace,
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
	ctrl.SetControllerReference(r.opniCluster, service, r.client.Scheme())
	return service
}

func (r *Reconciler) getReplicas() *int32 {
	if r.opniCluster.Spec.Nats.Replicas == nil {
		return pointer.Int32(3)
	}
	return r.opniCluster.Spec.Nats.Replicas
}

func (r *Reconciler) getNKeyUser() (string, error) {
	var seed []byte
	var err error
	var publicKey string
	secret := corev1.Secret{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats-secrets", r.opniCluster.Name),
		Namespace: r.opniCluster.Namespace,
	}, &secret)
	if k8serrors.IsNotFound(err) {
		user, err := nkeys.CreateUser()
		if err != nil {
			return "", err
		}
		seed, err = user.Seed()
		if err != nil {
			return "", err
		}
		publicKey, err = user.PublicKey()
		if err != nil {
			return "", err
		}

		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-secrets", r.opniCluster.Name),
				Namespace: r.opniCluster.Namespace,
			},
			Data: map[string][]byte{
				"seed": seed,
			},
		}
		ctrl.SetControllerReference(r.opniCluster, &secret, r.client.Scheme())
		err = r.client.Create(r.ctx, &secret)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	seed, ok := secret.Data["seed"]
	if !ok {
		user, err := nkeys.CreateUser()
		if err != nil {
			return "", err
		}
		seed, err = user.Seed()
		if err != nil {
			return "", err
		}
		publicKey, err = user.PublicKey()
		if err != nil {
			return "", err
		}

		secret.Data["seed"] = seed
		err = r.client.Update(r.ctx, &secret)
		if err != nil {
			return "", err
		}
	}

	if publicKey != "" {
		r.opniCluster.Status.NKeyUser = publicKey
		r.client.Status().Update(r.ctx, r.opniCluster)
		return publicKey, nil
	}

	if r.opniCluster.Status.NKeyUser == "" {
		user, err := nkeys.FromSeed(seed)
		if err != nil {
			return "", nil
		}
		r.opniCluster.Status.NKeyUser = publicKey
		r.client.Status().Update(r.ctx, r.opniCluster)
		return user.PublicKey()
	}

	return r.opniCluster.Status.NKeyUser, nil
}

// getNatsClusterPassword will check if there is already a nats cluster password stored in a secret
// and return the value.  If there is no secret it will generate a new password, store it in a secret,
// and then return the generated value
func (r *Reconciler) getNatsClusterPassword() ([]byte, error) {
	secret := corev1.Secret{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-nats-secrets", r.opniCluster.Name),
		Namespace: r.opniCluster.Namespace,
	}, &secret)
	if k8serrors.IsNotFound(err) {
		password := generateRandomPassword()

		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-nats-secrets", r.opniCluster.Name),
				Namespace: r.opniCluster.Namespace,
			},
			Data: map[string][]byte{
				"password": password,
			},
		}
		ctrl.SetControllerReference(r.opniCluster, &secret, r.client.Scheme())

		err := r.client.Create(r.ctx, &secret)
		if err != nil {
			return make([]byte, 0), err
		}

		return password, nil
	} else if err != nil {
		return make([]byte, 0), err
	}
	password, ok := secret.Data["password"]
	if !ok {
		password = generateRandomPassword()
		secret.Data["password"] = password
		err := r.client.Update(r.ctx, &secret)
		if err != nil {
			return make([]byte, 0), err
		}
	}
	return password, nil
}

func generateRandomPassword() []byte {
	rand.Seed(time.Now().UnixNano())
	chars := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return b
}
