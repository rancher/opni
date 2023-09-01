package opniopensearch

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"text/template"

	"github.com/Masterminds/semver"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/resources/multiclusterrolebinding"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	OpniPreprocessingInstanceName = "opni"
)

var (
	natsConnection = template.Must(template.New("natsconn").Parse(`nats:
  endpoint: nats://{{ .NatsName }}-nats-client.{{ .Namespace }}.svc:4222
  seed_file: /etc/nkey/seed
`))
)

type natsConfig struct {
	NatsName  string
	Namespace string
}

func (r *Reconciler) buildOpensearchCluster(
	natsAuthSecret string,
	certs certs.K8sOpensearchCertManager,
) *opsterv1.OpenSearchCluster {
	lg := log.FromContext(r.ctx)
	// Set default image version
	version := r.instance.Spec.Version
	if version == "unversioned" {
		version = "0.11.2"
	}

	image := calculateImage(r.instance.Spec.ImageRepo, version, r.instance.Spec.OpensearchVersion)

	updatedSecurityConfig := r.instance.Spec.OpensearchSettings.Security.DeepCopy()
	updatedSecurityConfig.Config = &opsterv1.SecurityConfig{
		SecurityconfigSecret: corev1.LocalObjectReference{
			Name: fmt.Sprintf("%s-securityconfig", r.instance.Name),
		},
		AdminCredentialsSecret: corev1.LocalObjectReference{
			Name: fmt.Sprintf("%s-internal-auth", r.instance.Name),
		},
	}

	// Set CA certs
	transportSecret, err := certs.GetTransportCARef()
	if err != nil {
		lg.Error(err, "failed to get transport ca")
	} else {
		updatedSecurityConfig.Tls.Transport.TlsCertificateConfig = opsterv1.TlsCertificateConfig{
			CaSecret: transportSecret,
		}
	}

	httpSecret, err := certs.GetHTTPCARef()
	if err != nil {
		lg.Error(err, "failed to get http ca")
	} else {
		updatedSecurityConfig.Tls.Http.TlsCertificateConfig = opsterv1.TlsCertificateConfig{
			CaSecret: httpSecret,
		}
	}

	updatedDashboards := r.instance.Spec.Dashboards.DeepCopy()
	updatedDashboards.OpensearchCredentialsSecret = corev1.LocalObjectReference{
		Name: fmt.Sprintf("%s-dashboards-auth", r.instance.Name),
	}

	cluster := &opsterv1.OpenSearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instance.Name,
			Namespace: r.instance.Namespace,
		},
		Spec: opsterv1.ClusterSpec{
			Bootstrap: opsterv1.BootstrapConfig{
				AdditionalConfig: map[string]string{
					"plugins.security.ssl.http.clientauth_mode": "OPTIONAL",
				},
			},
			General: opsterv1.GeneralConfig{
				ImageSpec: &opsterv1.ImageSpec{
					ImagePullPolicy: lo.ToPtr(corev1.PullAlways),
					Image: func() *string {
						if r.instance.Spec.OpensearchSettings.ImageOverride != nil {
							return r.instance.Spec.OpensearchSettings.ImageOverride
						}
						return &image
					}(),
				},
				Version:          r.instance.Spec.OpensearchVersion,
				ServiceName:      fmt.Sprintf("%s-opensearch-svc", r.instance.Name),
				HttpPort:         9200,
				SetVMMaxMapCount: true,
				AdditionalVolumes: func() []opsterv1.AdditionalVolume {
					if r.instance.Spec.NatsRef == nil {
						return []opsterv1.AdditionalVolume{}
					}
					return []opsterv1.AdditionalVolume{
						{
							Name: "nkey",
							Path: "/etc/nkey",
							Secret: &corev1.SecretVolumeSource{
								// TODO select this with labels (not hard coded)
								SecretName: natsAuthSecret,
							},
						},
						{
							Name: "pluginsettings",
							Path: "/usr/share/opensearch/config/preprocessing",
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: fmt.Sprintf("%s-nats-connection", r.instance.Name),
								},
							},
						},
					}
				}(),
				AdditionalConfig: r.additionalConfig(),
				PluginsList:      r.pluginsList(),
				Keystore:         r.keyStoreList(),
			},
			NodePools:  r.instance.Spec.NodePools,
			Security:   updatedSecurityConfig,
			Dashboards: *updatedDashboards,
		},
	}

	ctrl.SetControllerReference(r.instance, cluster, r.client.Scheme())
	return cluster
}

func (r *Reconciler) buildMulticlusterRoleBinding() runtime.Object {
	binding := &loggingv1beta1.MulticlusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instance.Name,
			Namespace: r.instance.Namespace,
		},
		Spec: loggingv1beta1.MulticlusterRoleBindingSpec{
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      r.instance.Name,
				Namespace: r.instance.Namespace,
			},
			OpensearchExternalURL: r.instance.Spec.ExternalURL,
			OpensearchConfig:      r.instance.Spec.ClusterConfigSpec,
			NeuralSearchSettings:  r.instance.Spec.OpensearchSettings.NeuralSearchSettings,
		},
	}
	ctrl.SetControllerReference(r.instance, binding, r.client.Scheme())
	return binding
}

func (r *Reconciler) buildConfigMap() runtime.Object {
	var buffer bytes.Buffer
	natsConnection.Execute(&buffer, natsConfig{
		NatsName:  r.instance.Spec.NatsRef.Name,
		Namespace: r.instance.Namespace,
	})
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-nats-connection", r.instance.Name),
			Namespace: r.instance.Namespace,
		},
		Data: map[string]string{
			"settings.yml": buffer.String(),
		},
	}
	ctrl.SetControllerReference(r.instance, configmap, r.client.Scheme())
	return configmap
}

func (r *Reconciler) buildOTELPreprocessor() runtime.Object {
	otel := &loggingv1beta1.Preprocessor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpniPreprocessingInstanceName,
			Namespace: r.instance.Namespace,
		},
		Spec: loggingv1beta1.PreprocessorSpec{
			ImageSpec: opnimeta.ImageSpec{
				ImagePullPolicy: lo.ToPtr(corev1.PullAlways),
			},
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      r.instance.Name,
				Namespace: r.instance.Namespace,
			},
			WriteIndex: multiclusterrolebinding.LogIndexAlias,
		},
	}
	ctrl.SetControllerReference(r.instance, otel, r.client.Scheme())
	return otel
}

func (r *Reconciler) buildS3Repository() *loggingv1beta1.OpensearchRepository {
	repo := &loggingv1beta1.OpensearchRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instance.Name,
			Namespace: r.instance.Namespace,
		},
	}
	if r.instance.Spec.OpensearchSettings.S3Settings != nil {
		repo.Spec = loggingv1beta1.OpensearchRepositorySpec{
			Settings: loggingv1beta1.RepositorySettings{
				S3: &r.instance.Spec.OpensearchSettings.S3Settings.Repository,
			},
			OpensearchClusterRef: &opnimeta.OpensearchClusterRef{
				Name:      r.instance.Name,
				Namespace: r.instance.Namespace,
			},
		}
	}
	ctrl.SetControllerReference(r.instance, repo, r.client.Scheme())
	return repo
}

func (r *Reconciler) fetchNatsAuthSecretName() (string, bool, error) {
	if r.instance.Spec.NatsRef == nil {
		return "", false, errors.New("missing nats reference")
	}
	nats := &opnicorev1beta1.NatsCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instance.Spec.NatsRef.Name,
			Namespace: r.instance.Namespace,
		},
	}
	err := r.client.Get(r.ctx, client.ObjectKeyFromObject(nats), nats)
	if err != nil {
		return "", false, err
	}

	if nats.Status.AuthSecretKeyRef == nil {
		return "", true, nil
	}

	return nats.Status.AuthSecretKeyRef.Name, false, nil
}

func calculateImage(repo, version, opensearchVersion string) string {
	newVersionConstraint, err := semver.NewConstraint(">=0.11.0-0")
	if err != nil {
		panic(err)
	}
	opniVersion, err := semver.NewVersion(version)
	if err != nil {
		return fmt.Sprintf(
			"%s/opensearch:v%s-%s",
			repo,
			version,
			opensearchVersion,
		)
	}
	if newVersionConstraint.Check(opniVersion) {
		return fmt.Sprintf(
			"%s/opensearch:v%s-%s",
			repo,
			version,
			opensearchVersion,
		)
	}
	return fmt.Sprintf(
		"%s/opensearch:%s-%s",
		repo,
		opensearchVersion,
		version,
	)
}

func (r *Reconciler) pluginsList() []string {
	plugins := []string{}
	if r.instance.Spec.S3Settings != nil {
		plugins = append(plugins, "repository-s3")
	}
	return plugins
}

func (r *Reconciler) keyStoreList() []opsterv1.KeystoreValue {
	keystores := []opsterv1.KeystoreValue{}
	if r.instance.Spec.S3Settings != nil {
		keystores = append(keystores, opsterv1.KeystoreValue{
			Secret: r.instance.Spec.S3Settings.CredentialSecret,
			KeyMappings: map[string]string{
				"accessKey": "s3.client.default.access_key",
				"secretKey": "s3.client.default.secret_key",
			},
		})
	}
	return keystores
}

func (r *Reconciler) additionalConfig() map[string]string {
	config := map[string]string{
		"plugins.security.ssl.http.clientauth_mode": "OPTIONAL",
	}
	if r.instance.Spec.S3Settings != nil {
		if r.instance.Spec.S3Settings.Endpoint != "" {
			config["s3.client.default.endpoint"] = r.instance.Spec.S3Settings.Endpoint
		}
		if r.instance.Spec.S3Settings.PathStyleAccess {
			config["s3.client.default.path_style_access"] = "true"
		}
		if r.instance.Spec.S3Settings.Protocol != "" {
			config["s3.client.default.protocol"] = string(r.instance.Spec.S3Settings.Protocol)
		}
		if r.instance.Spec.S3Settings.ProxyHost != "" {
			config["s3.client.default.proxy.host"] = r.instance.Spec.S3Settings.ProxyHost
		}
		if r.instance.Spec.S3Settings.ProxyPort != nil {
			config["s3.client.default.proxy.port"] = strconv.Itoa(int(lo.FromPtrOr(r.instance.Spec.S3Settings.ProxyPort, 443)))
		}
	}
	return config
}
