package apis

import (
	monitoringcoreosv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringcoreosv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
)

func init() {
	addSchemeBuilders(monitoringcoreosv1.AddToScheme)
	addSchemeBuilders(monitoringcoreosv1alpha1.AddToScheme)
}
