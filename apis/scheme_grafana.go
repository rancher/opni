//go:build !minimal

package apis

import opnigrafanav1alpha1 "github.com/rancher/opni/apis/grafana/v1alpha1"

func init() {
	addSchemeBuilders(opnigrafanav1alpha1.AddToScheme)
}
