//go:build !noscheme_prometheus && !noscheme_thirdparty

package apis

import monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

func init() {
	addSchemeBuilders(monitoringcoreosv1.AddToScheme)
}
