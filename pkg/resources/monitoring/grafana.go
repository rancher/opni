package monitoring

import (
	"encoding/json"
	"fmt"
	"strings"

	_ "embed"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	"github.com/imdario/mergo"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"github.com/ttacon/chalk"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:embed dashboards/dashboards.json
var dashboardsJson []byte

//go:embed dashboards/opni-gateway.json
var opniGatewayJson []byte

func (r *Reconciler) grafana() ([]resources.Resource, error) {
	dashboardSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			resources.AppNameLabel:  "grafana",
			resources.PartOfLabel:   "opni",
			resources.InstanceLabel: r.mc.Name,
		},
	}

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
	grafanaDashboards := []*grafanav1alpha1.GrafanaDashboard{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-gateway",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1alpha1.GrafanaDashboardSpec{
				Json: string(opniGatewayJson),
			},
		},
	}

	dashboards := map[string]json.RawMessage{}
	if err := json.Unmarshal(dashboardsJson, &dashboards); err != nil {
		return nil, err
	}
	for name, jsonData := range dashboards {
		grafanaDashboards = append(grafanaDashboards, &grafanav1alpha1.GrafanaDashboard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1alpha1.GrafanaDashboardSpec{
				Json: string(jsonData),
			},
		})
	}

	if !r.mc.Spec.Grafana.Enabled {
		absentResources := []resources.Resource{
			resources.Absent(grafana),
			resources.Absent(datasource),
		}
		for _, dashboard := range grafanaDashboards {
			absentResources = append(absentResources, resources.Absent(dashboard))
		}
		return absentResources, nil
	}

	grafanaHostname := fmt.Sprintf("grafana.%s", r.gw.Spec.Hostname)
	if r.mc.Spec.Grafana.Hostname != "" {
		grafanaHostname = r.mc.Spec.Grafana.Hostname
	}

	defaults := grafanav1alpha1.GrafanaSpec{
		DashboardLabelSelector: []*metav1.LabelSelector{dashboardSelector},
		BaseImage:              "grafana/grafana:latest",
		Client: &grafanav1alpha1.GrafanaClient{
			PreferService: util.Pointer(true),
		},
		Config: grafanav1alpha1.GrafanaConfig{
			Server: &grafanav1alpha1.GrafanaConfigServer{
				Domain:  grafanaHostname,
				RootUrl: "https://" + grafanaHostname,
			},
			Auth: &grafanav1alpha1.GrafanaConfigAuth{
				DisableLoginForm: util.Pointer(true),
			},
			AuthGenericOauth: &grafanav1alpha1.GrafanaConfigAuthGenericOauth{
				Enabled: util.Pointer(true),
				Scopes:  "openid profile email",
			},
			UnifiedAlerting: &grafanav1alpha1.GrafanaConfigUnifiedAlerting{
				Enabled: util.Pointer(true),
			},
			Alerting: &grafanav1alpha1.GrafanaConfigAlerting{
				Enabled: util.Pointer(false),
			},
		},
		Deployment: &grafanav1alpha1.GrafanaDeployment{
			SecurityContext: &corev1.PodSecurityContext{
				FSGroup: util.Pointer(int64(472)),
			},
		},
		Secrets: []string{"grafana-datasource-cert"},
		DataStorage: &grafanav1alpha1.GrafanaDataStorage{
			Size: resource.MustParse("10Gi"),
		},
	}

	spec := r.mc.Spec.Grafana.GrafanaSpec

	// apply defaults to user-provided config
	// ensure label selectors and secrets are appended to any user defined ones
	if err := mergo.Merge(&spec, defaults, mergo.WithAppendSlice); err != nil {
		return nil, err
	}

	// special case as we don't want the append slice logic for access modes
	if spec.DataStorage.AccessModes == nil {
		spec.DataStorage.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	// special case to fill the ingress hostname if not set
	if spec.Ingress != nil {
		if spec.Ingress.Hostname == "" {
			spec.Ingress.Hostname = grafanaHostname
		}
	}

	grafana.Spec = spec

	datasource.Spec = grafanav1alpha1.GrafanaDataSourceSpec{
		Name: "opni-monitoring-datasources",
		Datasources: []grafanav1alpha1.GrafanaDataSourceFields{
			{
				Name:            "Opni",
				Type:            "prometheus",
				Access:          "proxy",
				Url:             fmt.Sprintf("https://opni-monitoring-internal.%s.svc:8080/api/prom", r.mc.Namespace),
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
				Url:             fmt.Sprintf("https://opni-monitoring-internal.%s.svc:8080/api/prom", r.mc.Namespace),
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

	switch r.gw.Spec.Auth.Provider {
	case v1beta1.AuthProviderNoAuth:
		grafana.Spec.Config.AuthGenericOauth = &grafanav1alpha1.GrafanaConfigAuthGenericOauth{
			Enabled:           util.Pointer(true),
			ClientId:          "grafana",
			ClientSecret:      "noauth",
			Scopes:            "openid profile email",
			AuthUrl:           fmt.Sprintf("http://%s:4000/oauth2/authorize", r.gw.Spec.Hostname),
			TokenUrl:          fmt.Sprintf("http://%s:4000/oauth2/token", r.gw.Spec.Hostname),
			ApiUrl:            fmt.Sprintf("http://%s:4000/oauth2/userinfo", r.gw.Spec.Hostname),
			RoleAttributePath: "grafana_role",
		}
	case v1beta1.AuthProviderOpenID:
		spec := r.gw.Spec.Auth.Openid
		if spec.Discovery == nil && spec.WellKnownConfiguration == nil {
			return nil, openid.ErrMissingDiscoveryConfig
		}
		wkc, err := spec.OpenidConfig.GetWellKnownConfiguration()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch configuration from openid provider: %w", err)
		}
		scopes := spec.Scopes
		if len(scopes) == 0 {
			scopes = []string{"openid", "profile", "email"}
		}
		grafana.Spec.Config.AuthGenericOauth = &grafanav1alpha1.GrafanaConfigAuthGenericOauth{
			Enabled:               util.Pointer(true),
			ClientId:              spec.ClientID,
			ClientSecret:          spec.ClientSecret,
			Scopes:                strings.Join(scopes, " "),
			AuthUrl:               wkc.AuthEndpoint,
			TokenUrl:              wkc.TokenEndpoint,
			ApiUrl:                wkc.UserinfoEndpoint,
			RoleAttributePath:     spec.RoleAttributePath,
			AllowSignUp:           spec.AllowSignUp,
			AllowedDomains:        strings.Join(spec.AllowedDomains, " "),
			RoleAttributeStrict:   spec.RoleAttributeStrict,
			EmailAttributePath:    spec.EmailAttributePath,
			TLSSkipVerifyInsecure: spec.InsecureSkipVerify,
			TLSClientCert:         spec.TLSClientCert,
			TLSClientKey:          spec.TLSClientKey,
			TLSClientCa:           spec.TLSClientCA,
		}

		if spec.InsecureSkipVerify != nil && *spec.InsecureSkipVerify {
			r.logger.Warn(chalk.Yellow.Color("InsecureSkipVerify enabled for openid auth"))
		}
	}

	controllerutil.SetOwnerReference(r.mc, grafana, r.client.Scheme())
	controllerutil.SetOwnerReference(r.mc, datasource, r.client.Scheme())

	presentResources := []resources.Resource{
		resources.Present(grafana),
		resources.Present(datasource),
	}
	for _, dashboard := range grafanaDashboards {
		controllerutil.SetOwnerReference(r.mc, dashboard, r.client.Scheme())
		presentResources = append(presentResources, resources.Present(dashboard))
	}

	return presentResources, nil
}
