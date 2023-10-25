package monitoring

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	_ "embed"

	grafanav1beta1 "github.com/grafana-operator/grafana-operator/v5/api/v1beta1"
	"github.com/imdario/mergo"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
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

//go:embed dashboards/home.json
var homeDashboardJson []byte

//go:embed dashboards/opni-service-latency.json
var serviceLatencyDashboardJson []byte

//go:embed slo/slo_grafana_overview.json
var sloOverviewDashboard []byte

//go:embed slo/slo_grafana_detailed.json
var sloDetailedDashboard []byte

const (
	grafanaImageVersion = "10.1.1"
	grafanaImageRepo    = "grafana"
)

func (r *Reconciler) grafana() ([]resources.Resource, error) {
	dashboardSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			resources.AppNameLabel:  "grafana",
			resources.PartOfLabel:   "opni",
			resources.InstanceLabel: r.mc.Name,
		},
	}

	grafana := &grafanav1beta1.Grafana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni",
			Namespace: r.mc.Namespace,
			Labels:    dashboardSelector.MatchLabels,
		},
	}

	legacyResources := []resources.Resource{
		resources.Absent(&grafanav1beta1.Grafana{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-monitoring",
				Namespace: r.mc.Namespace,
			},
		}),
		resources.Absent(&grafanav1beta1.GrafanaDatasource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-monitoring",
				Namespace: r.mc.Namespace,
			},
		}),
	}

	opniDatasourceJSONCfg, err := createDatasourceJSONData()
	if err != nil {
		return nil, err
	}
	opniDatasourceSecureJSONCfg, err := createDatasourceSecureJSONData()
	if err != nil {
		return nil, err
	}

	opniDatasource := &grafanav1beta1.GrafanaDatasource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni",
			Namespace: r.mc.Namespace,
		},
		Spec: grafanav1beta1.GrafanaDatasourceSpec{
			Datasource: &grafanav1beta1.GrafanaDatasourceInternal{
				Name:   "Opni",
				Type:   "prometheus",
				Access: "proxy",
				URL:    fmt.Sprintf("https://opni-internal.%s.svc:8080/api/prom", r.mc.Namespace),
				// WithCredentials: lo.toPtr(true),
				Editable:       lo.ToPtr(false),
				IsDefault:      lo.ToPtr(true),
				JSONData:       opniDatasourceJSONCfg,
				SecureJSONData: opniDatasourceSecureJSONCfg,
			},
		},
	}

	opniAlertManagerDatasourceJSONCfg, err := createAlertManagerDatasourceJSONData()
	if err != nil {
		return nil, err
	}

	opniAlertManagerDatasource := &grafanav1beta1.GrafanaDatasource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni",
			Namespace: r.mc.Namespace,
		},
		Spec: grafanav1beta1.GrafanaDatasourceSpec{
			Datasource: &grafanav1beta1.GrafanaDatasourceInternal{
				Name:   "Opni Alertmanager",
				Type:   "opni_alertmanager",
				Access: "proxy",
				URL:    fmt.Sprintf("https://opni-internal.%s.svc:8080/api/prom", r.mc.Namespace),
				// WithCredentials: lo.toPtr(true),
				Editable:       lo.ToPtr(false),
				JSONData:       opniAlertManagerDatasourceJSONCfg,
				SecureJSONData: opniDatasourceSecureJSONCfg,
			},
		},
	}

	grafanaDashboards := []*grafanav1beta1.GrafanaDashboard{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-gateway.json",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(opniGatewayJson),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-home.json",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(homeDashboardJson),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
				//UseAsHomeDashboard: true,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-service-latency.json",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(serviceLatencyDashboardJson),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "slo-overview.json",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(sloOverviewDashboard),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "slo-detailed.json",
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(sloDetailedDashboard),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
			},
		},
	}

	dashboards := map[string]json.RawMessage{}
	if err := json.Unmarshal(dashboardsJson, &dashboards); err != nil {
		return nil, err
	}
	for name, jsonData := range dashboards {
		grafanaDashboards = append(grafanaDashboards, &grafanav1beta1.GrafanaDashboard{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: r.mc.Namespace,
				Labels:    dashboardSelector.MatchLabels,
			},
			Spec: grafanav1beta1.GrafanaDashboardSpec{
				Json: string(jsonData),
				InstanceSelector: &metav1.LabelSelector{
					MatchLabels: dashboardSelector.MatchLabels,
				},
			},
		})
	}

	if !r.mc.Spec.Grafana.GetEnabled() {
		absentResources := append([]resources.Resource{
			resources.Absent(grafana),
			resources.Absent(opniDatasource),
			resources.Absent(opniAlertManagerDatasource),
		}, legacyResources...)
		for _, dashboard := range grafanaDashboards {
			absentResources = append(absentResources, resources.Absent(dashboard))
		}
		return absentResources, nil
	}

	gatewayHostname := r.gw.Spec.Hostname
	gatewayAuthProvider := r.gw.Spec.Auth.Provider

	grafanaHostname := fmt.Sprintf("grafana.%s", gatewayHostname)
	if r.mc.Spec.Grafana.GetHostname() != "" {
		grafanaHostname = r.mc.Spec.Grafana.GetHostname()
	}

	if strings.Contains(grafanaHostname, "://") {
		_, grafanaHostname, _ = strings.Cut(grafanaHostname, "://")
	}

	grafanaHostname = strings.TrimSpace(grafanaHostname)
	if _, err := url.Parse(grafanaHostname); err != nil {
		return nil, fmt.Errorf("invalid grafana hostname: %w", err)
	}

	tag := grafanaImageVersion
	if r.mc.Spec.Grafana.GetVersion() != "" {
		tag = strings.TrimSpace(r.mc.Spec.Grafana.GetVersion())
	}

	defaults := grafanav1beta1.GrafanaSpec{
		Client: &grafanav1beta1.GrafanaClient{
			PreferIngress: lo.ToPtr(false),
		},
		Config: createDefaultGrafanaIni(grafanaHostname),
		Deployment: &grafanav1beta1.DeploymentV1{
			Spec: grafanav1beta1.DeploymentV1Spec{
				Template: &grafanav1beta1.DeploymentV1PodTemplateSpec{
					Spec: &grafanav1beta1.DeploymentV1PodSpec{
						Containers: []corev1.Container{
							{
								Image: grafanaImageRepo + "/grafana:" + tag,
								Env: []corev1.EnvVar{
									{
										Name:  "GF_INSTALL_PLUGINS",
										Value: "grafana-polystat-panel,marcusolsson-treemap-panel",
									},
								},
							},
						},
						SecurityContext: &corev1.PodSecurityContext{
							FSGroup: lo.ToPtr(int64(472)),
						},
					},
				},
			},
		},
		// Secrets: []string{"opni-gateway-client-cert"},
		PersistentVolumeClaim: &grafanav1beta1.PersistentVolumeClaimV1{
			Spec: &grafanav1beta1.PersistentVolumeClaimV1Spec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
		},
	}

	spec := r.mc.Spec.Grafana.GrafanaSpec

	// apply defaults to user-provided config
	// ensure label selectors and secrets are appended to any user defined ones
	if err := mergo.Merge(&spec, defaults, mergo.WithAppendSlice); err != nil {
		return nil, err
	}

	// special case as we don't want the append slice logic for access modes
	if spec.PersistentVolumeClaim.Spec.AccessModes == nil {
		spec.PersistentVolumeClaim.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	// special case to fill the ingress hostname if not set
	if spec.Ingress != nil && spec.Ingress.Spec != nil {
		for _, rule := range spec.Ingress.Spec.Rules {
			if rule.Host == "" {
				rule.Host = grafanaHostname
			}
		}
	}

	grafana.Spec = spec

	switch gatewayAuthProvider {
	case v1beta1.AuthProviderNoAuth:
		grafanaAuthGenericOauthCfg := map[string]string{
			"enabled":             "true",
			"client_id":           "grafana",
			"client_secret":       "noauth",
			"scopes":              "openid profile email",
			"auth_url":            fmt.Sprintf("http://%s:4000/oauth2/authorize", gatewayHostname),
			"token_url":           fmt.Sprintf("http://%s:4000/oauth2/token", gatewayHostname),
			"api_url":             fmt.Sprintf("http://%s:4000/oauth2/userinfo", gatewayHostname),
			"role_attribute_path": "grafana_role",
		}
		grafana.Spec.Config["auth.generic_oauth"] = grafanaAuthGenericOauthCfg

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
		grafanaAuthGenericOauthCfg := map[string]string{
			"enabled":                  "true",
			"client_id":                spec.ClientID,
			"client_secret":            spec.ClientSecret,
			"scopes":                   strings.Join(scopes, " "),
			"auth_url":                 wkc.AuthEndpoint,
			"token_url":                wkc.TokenEndpoint,
			"api_url":                  wkc.UserinfoEndpoint,
			"role_attribute_path":      spec.RoleAttributePath,
			"allow_sign_up":            strconv.FormatBool(lo.FromPtrOr(spec.AllowSignUp, false)),
			"allowed_domains":          strings.Join(spec.AllowedDomains, " "),
			"role_attribute_strict":    strconv.FormatBool(lo.FromPtrOr(spec.RoleAttributeStrict, false)),
			"email_attribute_path":     spec.EmailAttributePath,
			"tls_skip_verify_insecure": strconv.FormatBool(lo.FromPtrOr(spec.InsecureSkipVerify, false)),
			"tls_client_cert":          spec.TLSClientCert,
			"tls_client_key":           spec.TLSClientKey,
			"tls_client_ca":            spec.TLSClientCA,
		}

		grafana.Spec.Config["auth.generic_oauth"] = grafanaAuthGenericOauthCfg

		if wkc.EndSessionEndpoint != "" {
			grafana.Spec.Config["auth"]["signout_redirect_url"] = wkc.EndSessionEndpoint
		}

		if spec.InsecureSkipVerify != nil && *spec.InsecureSkipVerify {
			r.lg.Warn(chalk.Yellow.Color("InsecureSkipVerify enabled for openid auth"))
		}
	}

	controllerutil.SetOwnerReference(r.mc, grafana, r.client.Scheme())
	controllerutil.SetOwnerReference(r.mc, opniDatasource, r.client.Scheme())
	controllerutil.SetOwnerReference(r.mc, opniAlertManagerDatasource, r.client.Scheme())

	presentResources := []resources.Resource{
		resources.Present(grafana),
		resources.Present(opniDatasource),
		resources.Present(opniAlertManagerDatasource),
	}
	for _, dashboard := range grafanaDashboards {
		controllerutil.SetOwnerReference(r.mc, dashboard, r.client.Scheme())
		presentResources = append(presentResources, resources.Present(dashboard))
	}

	return append(presentResources, legacyResources...), nil
}

func createDefaultGrafanaIni(grafanaHostname string) map[string]map[string]string {
	config := make(map[string]map[string]string)

	serverSection := make(map[string]string)
	serverSection["domain"] = grafanaHostname
	serverSection["root_url"] = "https://" + grafanaHostname
	config["server"] = serverSection

	authSection := make(map[string]string)
	authSection["disable_login_form"] = "true"
	config["auth"] = authSection

	oauthSection := make(map[string]string)
	oauthSection["enabled"] = "true"
	oauthSection["scopes"] = "openid profile email"
	config["authGenericOauth"] = oauthSection

	unifiedAlertingSection := make(map[string]string)
	unifiedAlertingSection["enabled"] = "true"
	config["unified_alerting"] = unifiedAlertingSection

	alertingSection := make(map[string]string)
	alertingSection["enabled"] = "false"
	config["alerting"] = alertingSection

	featureTogglesSection := make(map[string]string)
	featureTogglesSection["enable"] = "accessTokenExpirationCheck panelTitleSearch increaseInMemDatabaseQueryCache newPanelChromeUI"
	config["feature_toggles"] = featureTogglesSection
	return config
}

func createDatasourceJSONData() (json.RawMessage, error) {
	datasourceCfg := make(map[string]any)
	datasourceCfg["alertmanagerUid"] = "opni_alertmanager"
	datasourceCfg["oauthPassThru"] = "true"
	datasourceCfg["TlsAuthWithCACert"] = "true"

	jsonData, err := json.Marshal(datasourceCfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jsonData), nil
}

func createAlertManagerDatasourceJSONData() (json.RawMessage, error) {
	datasourceCfg := make(map[string]any)
	datasourceCfg["implementation"] = "cortex"
	datasourceCfg["oauthPassThru"] = "true"
	datasourceCfg["TlsAuthWithCACert"] = "true"

	jsonData, err := json.Marshal(datasourceCfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jsonData), nil
}

func createDatasourceSecureJSONData() (json.RawMessage, error) {
	datasourceSecureCfg := make(map[string]any)
	datasourceSecureCfg["tlsCACert"] = "$__file{/etc/grafana-secrets/opni-gateway-client-cert/ca.crt}"
	datasourceSecureCfg["TlsClientCert"] = "$__file{/etc/grafana-secrets/opni-gateway-client-cert/tls.crt}"
	datasourceSecureCfg["TlsClientKey"] = "$__file{/etc/grafana-secrets/opni-gateway-client-cert/tls.key}"

	jsonData, err := json.Marshal(datasourceSecureCfg)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(jsonData), nil
}
