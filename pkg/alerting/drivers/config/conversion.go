package config

import (
	"net/url"

	amCfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

func ToWebhook(endp *alertingv1.WebhookEndpoint) *WebhookConfig {
	httpConfig := &commoncfg.HTTPClientConfig{}
	if hc := endp.GetHttpConfig(); hc != nil {
		if hc.GetAuthorization() != nil {
			httpConfig.Authorization = &commoncfg.Authorization{
				Type:        hc.GetAuthorization().GetType(),
				Credentials: commoncfg.Secret(hc.GetAuthorization().GetCredentials()),
			}
		}
		if hc.GetBasicAuth() != nil {
			httpConfig.BasicAuth = &commoncfg.BasicAuth{
				Username: hc.GetBasicAuth().GetUsername(),
				Password: commoncfg.Secret(hc.GetBasicAuth().GetPassword()),
			}
		}
		if hc.GetOauth2() != nil {
			httpConfig.OAuth2 = &commoncfg.OAuth2{
				ClientID:     hc.GetOauth2().GetClientId(),
				ClientSecret: commoncfg.Secret(hc.GetOauth2().GetClientSecret()),
				TokenURL:     hc.GetOauth2().GetTokenUrl(),
			}
		}
		if hc.GetProxyUrl() != "" {
			proxyUrl, err := url.Parse(hc.GetProxyUrl())
			if err != nil {
				panic(err)
			}
			httpConfig.ProxyURL = commoncfg.URL{URL: proxyUrl}
		}
		if hc.EnabledHttp2 {
			httpConfig.EnableHTTP2 = hc.EnabledHttp2
		}
		if hc.FollowRedirects {
			httpConfig.FollowRedirects = hc.FollowRedirects
		}
		if hc.GetTlsConfig() != nil {
			httpConfig.TLSConfig = commoncfg.TLSConfig{
				CAFile:             hc.GetTlsConfig().GetCaFile(),
				CertFile:           hc.GetTlsConfig().GetCertFile(),
				KeyFile:            hc.GetTlsConfig().GetKeyFile(),
				InsecureSkipVerify: hc.GetTlsConfig().GetInsecureSkipVerify(),
				ServerName:         hc.GetTlsConfig().GetServerName(),
			}
		}
	}
	webhookURL, err := url.Parse(endp.GetUrl())
	if err != nil {
		panic(err)
	}
	return &WebhookConfig{
		HTTPConfig: httpConfig,
		URL: &amCfg.URL{
			URL: webhookURL,
		},
		MaxAlerts: uint64(endp.MaxAlerts),
	}
}
