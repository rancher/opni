package alerting

import (
	// "fmt"
	// "net"
	// "strings"

	// "github.com/rancher/opni/pkg/alerting/shared"
	"bytes"
	"fmt"
	"path"
	"reflect"

	"github.com/cortexproject/cortex/pkg/alertmanager"
	"github.com/cortexproject/cortex/pkg/alertmanager/alertstore"
	"github.com/cortexproject/cortex/pkg/storage/bucket"
	"github.com/cortexproject/cortex/pkg/storage/bucket/azure"
	"github.com/cortexproject/cortex/pkg/storage/bucket/filesystem"
	"github.com/cortexproject/cortex/pkg/storage/bucket/gcs"
	"github.com/cortexproject/cortex/pkg/storage/bucket/s3"
	"github.com/cortexproject/cortex/pkg/storage/bucket/swift"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cortexproject/cortex/pkg/api"
	"github.com/cortexproject/cortex/pkg/cortex"
	"github.com/cortexproject/cortex/pkg/querier/tenantfederation"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ring/kv/memberlist"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	kyamlv3 "github.com/kralicky/yaml/v3"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	"github.com/weaveworks/common/logging"

	"github.com/weaveworks/common/server"

	// "github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	// "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	// "k8s.io/apimachinery/pkg/util/intstr"
	// ctrl "sigs.k8s.io/controller-runtime"
)

const (
	cortexAlertingSecret = "alerting-cortex-config"
	cortexConfigPath     = "/etc/cortex"
)

func (r *Reconciler) cortex() ([]resources.Resource, error) {
	cortexResources := []resources.Resource{}
	// cortexResources = append(cortexResources, r.cortexConfigMap())
	secret, err := r.cortexSecret()
	if err != nil {
		return cortexResources, err
	}
	cortexResources = append(cortexResources, secret)
	cortexResources = append(cortexResources, r.cortexSet()...)
	cortexResources = append(cortexResources, r.cortexService())
	return cortexResources, nil
}

func (r *Reconciler) cortexSecret() (resources.Resource, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cortexAlertingSecret,
			Namespace: r.gw.Namespace,
			Labels: map[string]string{
				resources.PartOfLabel: "opni",
			},
		},
		Data: map[string][]byte{},
	}
	if r.ac.Spec.Alertmanager.Enable {
		return resources.Absent(secret), nil
	}
	replicas := lo.FromPtrOr(r.ac.Spec.Alertmanager.ApplicationSpec.Replicas, 1)
	isHA := replicas > 1
	// prefix fallback path with data mount path const
	logLevel := logging.Level{}
	logLevel.Set("debug")
	logFmt := logging.Format{}
	logFmt.Set("json")
	kvConfig := kv.Config{
		Store: lo.Ternary(isHA, "memberlist", "inmemory"),
	}

	s3Spec := valueOrDefault(r.ac.Spec.Alertmanager.ApplicationSpec.GetS3())
	gcsSpec := valueOrDefault(r.ac.Spec.Alertmanager.ApplicationSpec.GetGcs())
	azureSpec := valueOrDefault(r.ac.Spec.Alertmanager.ApplicationSpec.GetAzure())
	swiftSpec := valueOrDefault(r.ac.Spec.Alertmanager.ApplicationSpec.GetSwift())

	// TODO : need to assign things from the crd spec to the alertmanager configuration
	// TODO : figure out how to pass memeberlist to args
	storageConfig := bucket.Config{
		Backend: string(r.ac.Spec.Alertmanager.ApplicationSpec.GetBackend()),
		S3: s3.Config{
			Endpoint:   s3Spec.GetEndpoint(),
			Region:     s3Spec.GetRegion(),
			BucketName: s3Spec.GetBucketName(),
			SecretAccessKey: flagext.Secret{
				Value: s3Spec.GetSecretAccessKey(),
			},
			AccessKeyID:      s3Spec.GetAccessKeyID(),
			Insecure:         s3Spec.GetInsecure(),
			SignatureVersion: s3Spec.GetSignatureVersion(),
			SSE: s3.SSEConfig{
				Type:                 s3Spec.GetSse().GetType(),
				KMSKeyID:             s3Spec.GetSse().GetKmsKeyID(),
				KMSEncryptionContext: s3Spec.GetSse().GetKmsEncryptionContext(),
			},
			HTTP: s3.HTTPConfig{
				Config: bucketHttpConfig(s3Spec.GetHttp()),
			},
		},
		GCS: gcs.Config{
			BucketName: gcsSpec.GetBucketName(),
			ServiceAccount: flagext.Secret{
				Value: gcsSpec.GetServiceAccount(),
			},
		},
		Azure: azure.Config{
			StorageAccountName: azureSpec.GetStorageAccountName(),
			StorageAccountKey: flagext.Secret{
				Value: azureSpec.GetStorageAccountKey(),
			},
			ContainerName: azureSpec.GetContainerName(),
			Endpoint:      azureSpec.GetEndpoint(),
			MaxRetries:    int(azureSpec.GetMaxRetries()),
			Config:        bucketHttpConfig(azureSpec.GetHttp()),
		},
		Swift: swift.Config{
			AuthVersion:       int(swiftSpec.GetAuthVersion()),
			AuthURL:           swiftSpec.GetAuthURL(),
			Username:          swiftSpec.GetUsername(),
			UserDomainName:    swiftSpec.GetUserDomainName(),
			UserDomainID:      swiftSpec.GetUserDomainID(),
			UserID:            swiftSpec.GetUserID(),
			Password:          swiftSpec.GetPassword(),
			DomainID:          swiftSpec.GetDomainID(),
			DomainName:        swiftSpec.GetDomainName(),
			ProjectID:         swiftSpec.GetProjectID(),
			ProjectName:       swiftSpec.GetProjectName(),
			ProjectDomainID:   swiftSpec.GetProjectDomainID(),
			ProjectDomainName: swiftSpec.GetProjectDomainName(),
			RegionName:        swiftSpec.GetRegionName(),
			ContainerName:     swiftSpec.GetContainerName(),
			MaxRetries:        int(swiftSpec.GetMaxRetries()),
			ConnectTimeout:    swiftSpec.GetConnectTimeout().AsDuration(),
			RequestTimeout:    swiftSpec.GetRequestTimeout().AsDuration(),
		},
		Filesystem: filesystem.Config{
			Directory: dataMountPath,
		},
	}

	config := cortex.Config{
		TenantFederation: tenantfederation.Config{
			Enabled: true,
		},
		API: api.Config{
			AlertmanagerHTTPPrefix: "/alertmanager",
			ResponseCompression:    false,
		},
		Server: server.Config{
			HTTPListenPort:                 8080,
			GRPCListenPort:                 9095,
			GPRCServerMaxConcurrentStreams: 10000,
			GRPCServerMaxSendMsgSize:       100 << 20,
			GPRCServerMaxRecvMsgSize:       100 << 20, // typo in upstream
			LogLevel:                       logLevel,
			LogFormat:                      logFmt,
		},
		MemberlistKV: memberlist.KVConfig{
			JoinMembers: lo.Ternary(isHA, flagext.StringSlice{"cortex-memberlist"}, nil),
		},
		Alertmanager: alertmanager.MultitenantAlertmanagerConfig{
			EnableAPI:       true,
			ShardingEnabled: isHA,
			ShardingRing: alertmanager.RingConfig{
				KVStore:           kvConfig,
				ReplicationFactor: int(replicas),
			},
			FallbackConfigFile: path.Join(cortexConfigPath, fallbackKey)},
		AlertmanagerStorage: alertstore.Config{
			Config: storageConfig,
		},
	}

	buf := new(bytes.Buffer)
	encoder := kyamlv3.NewEncoder(buf)
	encoder.SetAlwaysOmitEmpty(true)
	encoder.OverrideMarshalerForType(reflect.TypeOf(flagext.Secret{}),
		newOverrideMarshaler(func(s flagext.Secret) (any, error) {
			return s.Value, nil
		}),
	)
	err := encoder.Encode(config)
	if err != nil {
		return nil, err
	}

	// TODO : assign fallback configuration
	secret.Data[fallbackKey] = []byte(`
global: {}
  templates: []
  route:
    receiver: default
    receivers:
    - name: default
  inhibit_rules: []
  mute_time_intervals: []
 `)

	secret.Data["cortex.yaml"] = buf.Bytes()
	ctrl.SetControllerReference(r.ac, secret, r.client.Scheme())
	return resources.PresentIff(r.ac.Spec.Alertmanager.Enable, secret), nil
}

func (r *Reconciler) cortexSet() []resources.Resource {
	pvc, requiredVolumes := r.handlePVC("5Gi") // data path
	requiredVolumes = append(requiredVolumes, []corev1.Volume{
		{
			Name: "cortex-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cortexAlertingSecret,
					Items: []corev1.KeyToPath{
						{
							Key:  "cortex.yaml",
							Path: "cortex.yaml",
						},
						{
							Key:  "fallback.yaml",
							Path: "fallback.yaml",
						},
					},
				},
			},
		},
	}...)
	requiredVolumeMounts := []corev1.VolumeMount{
		{
			Name:      requiredData,
			MountPath: dataMountPath,
		},
		{
			Name:      "cortex-config",
			MountPath: cortexConfigPath,
		},
	}
	requiredPersistentClaims := []corev1.PersistentVolumeClaim{pvc}

	// iterate over CRD configs
	for _, vSpec := range r.ac.Spec.Alertmanager.ApplicationSpec.ExtraVolumes {
		requiredVolumes = append(requiredVolumes, vSpec.Volume)
		requiredVolumeMounts = append(requiredVolumeMounts, vSpec.VolumeMount)
	}

	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: r.gw.Namespace,
			Labels: map[string]string{
				resources.PartOfLabel:    "opni",
				"app.kubernetes.io/name": "opni-alerting",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: r.ac.Spec.Alertmanager.ApplicationSpec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					resources.PartOfLabel:    "opni",
					"app.kubernetes.io/name": "opni-alerting",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						resources.PartOfLabel:    "opni",
						"app.kubernetes.io/name": "opni-alerting",
					},
				},
				Spec: corev1.PodSpec{
					Affinity: r.ac.Spec.Alertmanager.ApplicationSpec.Affinity,
					Containers: []corev1.Container{
						{
							Env: append(
								[]corev1.EnvVar{
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
								r.ac.Spec.Alertmanager.ApplicationSpec.ExtraEnvVars...,
							),
							Name:  "opni-alertmanager",
							Image: r.gw.Status.Image,
							Args: append([]string{
								"cortex",
								"--target=alertmanager",
								fmt.Sprintf("--config.file=%scortex.yaml", cortexConfigPath),
							}, []string{}...),
							Ports: []corev1.ContainerPort{
								// TODO : we need to embed the opni-webhook
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
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "alertmanager/-/ready",
										Port: intstr.FromString("web-port"),
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "alertmanager/-/healthy",
										Port: intstr.FromString("web-port"),
									},
								},
							},
							VolumeMounts: requiredVolumeMounts,
							Resources: lo.FromPtrOr(
								r.ac.Spec.Alertmanager.ApplicationSpec.ResourceRequirements,
								corev1.ResourceRequirements{},
							),
						},
						{
							Env: append(
								[]corev1.EnvVar{
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
								r.ac.Spec.Alertmanager.ApplicationSpec.ExtraEnvVars...,
							),
							Name:            "opni-syncer",
							Image:           r.gw.Status.Image,
							ImagePullPolicy: "Always",
							Args: []string{
								"alerting-server",
								"syncer",
								"--syncer.alertmanager.address",
								"localhost:9093",
								"--syncer.gateway.join.address",
								r.gw.Spec.Management.GetGRPCListenAddress(),
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "syncer-port",
									ContainerPort: 8080,
									Protocol:      "TCP",
								},
							},
							VolumeMounts: requiredVolumeMounts,
						},
					},
					Volumes: requiredVolumes,
				},
			},
			VolumeClaimTemplates: requiredPersistentClaims,
		},
	}
	ctrl.SetControllerReference(r.ac, ss, r.client.Scheme())
	return []resources.Resource{
		resources.PresentIff(r.ac.Spec.Alertmanager.Enable, ss),
	}
}

func (r *Reconciler) cortexService() resources.Resource {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-alerting",
			Namespace: r.gw.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				resources.PartOfLabel:    "opni",
				"app.kubernetes.io/name": "opni-alerting",
			},
			Ports: []corev1.ServicePort{
				{
					Name: "web-port",
					Port: 9093,
				},
			},
		},
	}
	ctrl.SetControllerReference(r.ac, svc, r.client.Scheme())
	return resources.PresentIff(r.ac.Spec.Alertmanager.Enable, svc)
}
