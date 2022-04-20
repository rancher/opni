package monitoring

import (
	"fmt"
	"strings"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) grafana() ([]resources.Resource, error) {
	grafana := &grafanav1alpha1.Grafana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.mc.Namespace,
		},
	}
	datasource := &grafanav1alpha1.GrafanaDataSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.mc.Namespace,
		},
	}

	if !r.mc.Spec.Grafana.Enabled {
		return []resources.Resource{
			resources.Absent(grafana),
			resources.Absent(datasource),
		}, nil
	}

	baseImage := r.mc.Spec.Grafana.Image
	if baseImage == "" {
		baseImage = "grafana/grafana:latest"
	}
	grafana.Spec = grafanav1alpha1.GrafanaSpec{
		BaseImage: baseImage,
		Config: grafanav1alpha1.GrafanaConfig{
			Log: &grafanav1alpha1.GrafanaConfigLog{
				Level: r.mc.Spec.Grafana.LogLevel,
			},
			Server: &grafanav1alpha1.GrafanaConfigServer{
				Domain:  fmt.Sprintf("grafana.%s", r.mc.Spec.Gateway.Hostname),
				RootUrl: fmt.Sprintf("https://grafana.%s", r.mc.Spec.Gateway.Hostname),
			},
			Auth: &grafanav1alpha1.GrafanaConfigAuth{
				DisableLoginForm: util.Pointer(true),
			},
			AuthGenericOauth: &grafanav1alpha1.GrafanaConfigAuthGenericOauth{
				Enabled: util.Pointer(true),
				Scopes:  "openid profile email",
			},
			AuthProxy: &grafanav1alpha1.GrafanaConfigAuthProxy{
				Enabled: util.Pointer(true),
			},
			AuthBasic: &grafanav1alpha1.GrafanaConfigAuthBasic{
				Enabled: util.Pointer(false),
			},
			UnifiedAlerting: &grafanav1alpha1.GrafanaConfigUnifiedAlerting{
				Enabled: util.Pointer(true),
			},
			Alerting: &grafanav1alpha1.GrafanaConfigAlerting{
				Enabled: util.Pointer(false),
			},
		},
		Ingress: &grafanav1alpha1.GrafanaIngress{
			Enabled:       true,
			Hostname:      fmt.Sprintf("grafana.%s", r.mc.Spec.Gateway.Hostname),
			TLSEnabled:    true,
			TLSSecretName: "grafana-dashboard-tls",
			Path:          "/",
			PathType:      "Prefix",
		},
		Secrets: []string{"grafana-datasource-cert"},
		DataStorage: &grafanav1alpha1.GrafanaDataStorage{
			Size: resource.MustParse("10Gi"),
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
		},
	}

	datasource.Spec = grafanav1alpha1.GrafanaDataSourceSpec{
		Name: "opni-monitoring-datasources",
		Datasources: []grafanav1alpha1.GrafanaDataSourceFields{
			{
				Name:            "Opni",
				Type:            "prometheus",
				Access:          "proxy",
				Url:             fmt.Sprintf("https://opni-monitoring.%s.svc:8080/api/prom", r.mc.Namespace),
				WithCredentials: true,
				Editable:        false,
				IsDefault:       true,
				JsonData: grafanav1alpha1.GrafanaDataSourceJsonData{
					AlertManagerUID:   "opni_alertmanager",
					OauthPassThru:     true,
					TlsAuthWithCACert: true,
				},
				SecureJsonData: grafanav1alpha1.GrafanaDataSourceSecureJsonData{
					TlsCaCert: "$__file{/etc/grafana-secrets/grafana-datasource-cert/ca.crt}",
				},
			},
			{
				Name:            "Opni Alertmanager",
				Uid:             "opni_alertmanager",
				Type:            "alertmanager",
				Access:          "proxy",
				Url:             fmt.Sprintf("https://opni-monitoring.%s.svc:8080/api/prom", r.mc.Namespace),
				WithCredentials: true,
				Editable:        false,
				JsonData: grafanav1alpha1.GrafanaDataSourceJsonData{
					Implementation:    "cortex",
					TlsAuthWithCACert: true,
					OauthPassThru:     true,
				},
				SecureJsonData: grafanav1alpha1.GrafanaDataSourceSecureJsonData{
					TlsCaCert: "$__file{/etc/grafana-secrets/grafana-datasource-cert/ca.crt}",
				},
			},
		},
	}

	switch r.mc.Spec.Gateway.Auth.Provider {
	case v1beta1.AuthProviderNoAuth:
		grafana.Spec.Config.AuthGenericOauth.ClientId = "grafana"
		grafana.Spec.Config.AuthGenericOauth.ClientSecret = "noauth"
		grafana.Spec.Config.AuthGenericOauth.AuthUrl = fmt.Sprintf("http://%s:4000/oauth2/authorize", r.mc.Spec.Gateway.Hostname)
		grafana.Spec.Config.AuthGenericOauth.TokenUrl = fmt.Sprintf("http://%s:4000/oauth2/token", r.mc.Spec.Gateway.Hostname)
		grafana.Spec.Config.AuthGenericOauth.ApiUrl = fmt.Sprintf("http://%s:4000/oauth2/userinfo", r.mc.Spec.Gateway.Hostname)
		grafana.Spec.Config.AuthGenericOauth.RoleAttributePath = "grafana_role"
	case v1beta1.AuthProviderOpenID:
		spec := r.mc.Spec.Gateway.Auth.Openid
		if spec.Discovery == nil && spec.WellKnownConfiguration == nil {
			return nil, openid.ErrMissingDiscoveryConfig
		}
		wkc, err := spec.OpenidConfig.GetWellKnownConfiguration()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch configuration from openid provider: %w", err)
		}

		grafana.Spec.Config.AuthGenericOauth.ClientId = spec.ClientID
		grafana.Spec.Config.AuthGenericOauth.ClientSecret = spec.ClientSecret
		grafana.Spec.Config.AuthGenericOauth.Scopes = strings.Join(spec.Scopes, " ")
		grafana.Spec.Config.AuthGenericOauth.AuthUrl = wkc.AuthEndpoint
		grafana.Spec.Config.AuthGenericOauth.TokenUrl = wkc.TokenEndpoint
		grafana.Spec.Config.AuthGenericOauth.ApiUrl = wkc.UserinfoEndpoint
		grafana.Spec.Config.AuthGenericOauth.RoleAttributePath = spec.RoleAttributePath

		if spec.InsecureSkipVerify != nil && *spec.InsecureSkipVerify {
			r.logger.Warn("InsecureSkipVerify enabled for openid auth")
			grafana.Spec.Config.AuthGenericOauth.TLSSkipVerifyInsecure = spec.InsecureSkipVerify
		}
	}

	controllerutil.SetOwnerReference(r.mc, grafana, r.client.Scheme())
	controllerutil.SetOwnerReference(r.mc, datasource, r.client.Scheme())

	return []resources.Resource{
		resources.Present(grafana),
		resources.Present(datasource),
	}, nil
}
