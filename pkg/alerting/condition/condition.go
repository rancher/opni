package condition

import (
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
)

var OpniDisconnect *alertingv1alpha.AlertCondition = &alertingv1alpha.AlertCondition{
	Name:        "Disconnected Opni Agent {{ .agentId }} ",
	Description: "Opni agent {{ .agentId }} has been disconnected for more than {{ .timeout }}",
	Labels:      []string{"opni", "agent", "system"},
	Severity:    alertingv1alpha.Severity_CRITICAL,
	AlertType:   &alertingv1alpha.AlertCondition_System{},
}
