package v1

import (
	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
)

func init() {
	SchemeBuilder.Register(
		&grafanav1alpha1.Grafana{}, &grafanav1alpha1.GrafanaList{},
		&grafanav1alpha1.GrafanaDashboard{}, &grafanav1alpha1.GrafanaDashboardList{},
		&grafanav1alpha1.GrafanaDataSource{}, &grafanav1alpha1.GrafanaDataSourceList{},
		&grafanav1alpha1.GrafanaNotificationChannel{}, &grafanav1alpha1.GrafanaNotificationChannelList{},
	)
}
