package v1

import (
	grafanav1beta1 "github.com/grafana-operator/grafana-operator/v5/api/v1beta1"
)

func init() {
	SchemeBuilder.Register(
		&grafanav1beta1.Grafana{}, &grafanav1beta1.GrafanaList{},
		&grafanav1beta1.GrafanaDashboard{}, &grafanav1beta1.GrafanaDashboardList{},
		&grafanav1beta1.GrafanaDatasource{}, &grafanav1beta1.GrafanaDatasourceList{},
		&grafanav1beta1.GrafanaFolder{}, &grafanav1beta1.GrafanaFolderList{},
	)
}
