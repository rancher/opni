package opniopensearch

import (
	"bytes"
	"fmt"
	"html/template"

	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	opsterv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (r *Reconciler) buildOpensearchCluster(natsAuthSecret string) *opsterv1.OpenSearchCluster {
	// Set default image version
	version := r.instance.Spec.Version
	if version == "unversioned" {
		version = "0.6.0"
	}

	image := fmt.Sprintf(
		"%s/opensearch:%s-%s",
		r.instance.Spec.ImageRepo,
		r.instance.Spec.OpensearchVersion,
		version,
	)
	cluster := &opsterv1.OpenSearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.instance.Name,
			Namespace: r.instance.Namespace,
		},
		Spec: opsterv1.ClusterSpec{
			General: opsterv1.GeneralConfig{
				ImageSpec: &opsterv1.ImageSpec{
					Image:           &image,
					ImagePullPolicy: lo.ToPtr(corev1.PullAlways),
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
			},
			NodePools:  r.instance.Spec.NodePools,
			Security:   r.instance.Spec.OpensearchSettings.Security,
			Dashboards: r.instance.Spec.Dashboards,
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

func (r *Reconciler) fetchNatsAuthSecretName() (string, bool, error) {
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
