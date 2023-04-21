//go:build minimal

package apis

import (
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
)

func init() {
	addSchemeBuilders(
		opniloggingv1beta1.AddToScheme,
	)
}
