package gateway

import (
	"fmt"

	"emperror.dev/errors"
	"github.com/mitchellh/mapstructure"
	cfgmeta "github.com/rancher/opni-monitoring/pkg/config/meta"
	cfgv1beta1 "github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/sdk/resources"
	"github.com/rancher/opni-monitoring/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/yaml"
)

func (r *Reconciler) configMap() (resources.Resource, error) {
	gatewayConf := &cfgv1beta1.GatewayConfig{
		TypeMeta: cfgmeta.TypeMeta{
			Kind:       "GatewayConfig",
			APIVersion: "v1beta1",
		},
		Spec: cfgv1beta1.GatewayConfigSpec{
			ListenAddress: ":8080",
			Cortex: cfgv1beta1.CortexSpec{
				Certs: cfgv1beta1.MTLSSpec{
					ServerCA:   "/run/cortex/certs/server/ca.crt",
					ClientCA:   "/run/cortex/certs/client/ca.crt",
					ClientCert: "/run/cortex/certs/client/tls.crt",
					ClientKey:  "/run/cortex/certs/client/tls.key",
				},
			},
			AuthProvider: string(r.gateway.Spec.Auth.Provider),
			Certs: cfgv1beta1.CertsSpec{
				CACert:      util.Pointer("/run/opni-monitoring/certs/ca.crt"),
				ServingCert: util.Pointer("/run/opni-monitoring/certs/tls.crt"),
				ServingKey:  util.Pointer("/run/opni-monitoring/certs/tls.key"),
			},
			Storage: cfgv1beta1.StorageSpec{
				Type: cfgv1beta1.StorageTypeCRDs,
				CustomResources: &cfgv1beta1.CustomResourcesStorageSpec{
					Namespace: r.gateway.Namespace,
				},
			},
		},
	}

	var apSpec cfgv1beta1.AuthProviderSpec
	switch t := cfgv1beta1.AuthProviderType(r.gateway.Spec.Auth.Provider); t {
	case cfgv1beta1.AuthProviderOpenID:
		apSpec.Type = cfgv1beta1.AuthProviderOpenID
		if err := mapstructure.Decode(r.gateway.Spec.Auth.Openid, &apSpec.Options); err != nil {
			return nil, errors.WrapIf(err, "failed to decode openid auth provider options")
		}
	case cfgv1beta1.AuthProviderNoAuth:
		apSpec.Type = cfgv1beta1.AuthProviderNoAuth
		if err := mapstructure.Decode(r.gateway.Spec.Auth.Noauth, &apSpec.Options); err != nil {
			return nil, errors.WrapIf(err, "failed to decode noauth auth provider options")
		}
	default:
		return nil, errors.Errorf("unsupported auth provider: %s", t)
	}

	authProvider := &cfgv1beta1.AuthProvider{
		TypeMeta: cfgmeta.TypeMeta{
			Kind:       "AuthProvider",
			APIVersion: "v1beta1",
		},
		ObjectMeta: cfgmeta.ObjectMeta{
			Name: string(r.gateway.Spec.Auth.Provider),
		},
		Spec: apSpec,
	}

	gatewayConfData, err := yaml.Marshal(gatewayConf)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal gateway config")
	}
	authProviderData, err := yaml.Marshal(authProvider)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal auth provider")
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.gateway.Namespace,
			Labels:    resources.Labels(),
		},
		Data: map[string]string{
			"config.yaml": fmt.Sprintf("%s\n---\n%s", gatewayConfData, authProviderData),
		},
	}
	ctrl.SetControllerReference(r.gateway, cm, r.client.Scheme())
	return resources.Present(cm), nil
}
