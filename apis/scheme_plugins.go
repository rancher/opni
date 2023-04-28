//go:build !minimal

package apis

import (
	opniaiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opnimonitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
)

func init() {
	addSchemeBuilders(
		opniaiv1beta1.AddToScheme,
		opniloggingv1beta1.AddToScheme,
		opnimonitoringv1beta1.AddToScheme,
	)
}
