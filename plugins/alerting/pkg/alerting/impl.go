package alerting

import (
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AlertTypeSystemBackend struct {
	RequestBase
}

func (a *AlertTypeSystemBackend) ListTemplates() []TemplateInfo {
	return []TemplateInfo{}
}

func (a *AlertTypeSystemBackend) ListChoices() error {
	return nil
}

func (a *AlertTypeSystemBackend) Create(p *Plugin, req alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	return nil, nil
}

func (a *AlertTypeSystemBackend) Update(p *Plugin, req alertingv1alpha.AlertConditionWithId) (*emptypb.Empty, error) {
	return nil, nil
}

func (a *AlertTypeSystemBackend) Delete(p *Plugin, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, nil
}

type AlertTypeKubeMetricsBackend struct {
	RequestBase
}

func (a *AlertTypeKubeMetricsBackend) ListTemplates() []TemplateInfo {
	return nil
}

func (a *AlertTypeKubeMetricsBackend) ListChoices() error {
	return nil
}

func (a *AlertTypeKubeMetricsBackend) Create(p *Plugin, req alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	return nil, nil
}

func (a *AlertTypeKubeMetricsBackend) Update(p *Plugin, req alertingv1alpha.AlertConditionWithId) (*emptypb.Empty, error) {
	return nil, nil
}

func (a *AlertTypeKubeMetricsBackend) Delete(p *Plugin, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, nil
}
