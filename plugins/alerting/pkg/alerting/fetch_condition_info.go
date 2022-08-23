package alerting

import (
	"context"
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
)

func handleChoicesByType(
	p *Plugin,
	ctx context.Context,
	req *alertingv1alpha.AlertDetailChoicesRequest,
) (*alertingv1alpha.ListAlertTypeDetails, error) {
	if req.GetAlertType() == alertingv1alpha.AlertType_SYSTEM {
		return nil, shared.AlertingErrNotImplemented
	}
	if req.GetAlertType() == alertingv1alpha.AlertType_KUBE_STATE {
		return fetchKubeStateInfo(p, ctx)
	}
	return nil, shared.AlertingErrNotImplemented
}

func fetchKubeStateInfo(p *Plugin, ctx context.Context) (*alertingv1alpha.ListAlertTypeDetails, error) {
	resKubeState := &alertingv1alpha.ListAlertConditionKubeState{
		Objects: []string{},
		States:  []string{},
	}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	for _, cl := range clusters.Items {
		labelSets, err := p.adminClient.Get().GetMetricLabelSets(ctx, &cortexadmin.LabelRequest{
			Tenant:     cl.Id,
			JobId:      "",
			MetricName: metrics.KubePodStatusMetricName,
		})
		if err != nil {
			return nil, err
		}
		for _, l := range labelSets.Items {
			if l.GetName() == "pod" {
				resKubeState.Objects = append(resKubeState.Objects, l.GetItems()...)
			}
		}
	}
	for _, state := range metrics.KubePodStates {
		resKubeState.States = append(resKubeState.States, state)
	}

	return &alertingv1alpha.ListAlertTypeDetails{
		Type: &alertingv1alpha.ListAlertTypeDetails_KubeState{
			KubeState: resKubeState,
		},
	}, nil
}
