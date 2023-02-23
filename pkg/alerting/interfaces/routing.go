package interfaces

import (
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

type Routable interface {
	GetRoutingLabels() map[string]string
	GetRoutingAnnotations() map[string]string
	GetRoutingGoldenSignal() string
}

var _ Routable = (*alertingv1.AlertCondition)(nil)
var _ Routable = (*alertingv1.Notification)(nil)
