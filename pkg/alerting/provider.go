package alerting

import (
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
)

// alerting interface to be injected into the gateway
//
// Should at least encapsulate all alerting plugin implementations
type Provider interface {
	alertingv1alpha.AlertingClient
}
