package alerting

import (
	"context"
	"time"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/tidwall/gjson"
)

func handleChoicesByType(
	p *Plugin,
	ctx context.Context,
	req *alertingv1.AlertDetailChoicesRequest,
) (*alertingv1.ListAlertTypeDetails, error) {
	if req.GetAlertType() == alertingv1.AlertType_SYSTEM {
		return fetchAgentInfo(p, ctx)
	}
	if req.GetAlertType() == alertingv1.AlertType_KUBE_STATE {
		return fetchKubeStateInfo(p, ctx)
	}
	return nil, shared.AlertingErrNotImplemented
}
func clusterHasKubeStateMetrics(adminClient cortexadmin.CortexAdminClient, cl *corev1.Cluster) bool {
	return true
}

func fetchAgentInfo(p *Plugin, ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	ctxCa, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	mgmtClient, err := p.mgmtClient.GetContext(ctxCa)
	if err != nil {
		return nil, err
	}
	clusters, err := mgmtClient.ListClusters(ctxCa, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	resSystem := &alertingv1.ListAlertConditionSystem{
		AgentIds: []string{},
	}
	for _, cl := range clusters.Items {
		resSystem.AgentIds = append(resSystem.AgentIds, cl.Id)
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_System{
			System: resSystem,
		},
	}, nil
}

func fetchKubeStateInfo(p *Plugin, ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchKubeStateInfo")
	resKubeState := &alertingv1.ListAlertConditionKubeState{
		Clusters: map[string]*alertingv1.KubeObjectGroups{},
		States:   metrics.KubeStates,
	}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("Failed to get cortex client %s", err)
	}
	for _, cl := range clusters.Items {
		if clusterHasKubeStateMetrics(adminClient, cl) {
			rawKubeStateSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.KubeObjMetricNameMatcher,
			})
			if err != nil {
				lg.Warnf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.KubeObjMetricNameMatcher, err)
				continue
			}
			result := gjson.Get(string(rawKubeStateSeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warnf("no result kube state metrics found for cluster %s : %s", cl.Id, result)
				continue
			}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				// value array is [timestamp int, value int]
				seriesTsAndValue := series.Get("value")

				if !seriesInfo.Exists() || !seriesTsAndValue.Exists() {
					lg.Warnf("Series info or value does not exist %s", series)
					continue
				}
				value := seriesTsAndValue.Array()[1].Int()
				if value == 0 {
					continue
				}
				seriesInfoMap := seriesInfo.Map()
				name := seriesInfoMap["__name__"].String()
				namespace := seriesInfoMap["namespace"].String()
				//phase := seriesInfo["phase"].String()

				objType := metrics.KubeObjTypeExtractor.FindStringSubmatch(name)[1]
				if objType == "namespace" {
					continue // namespaces don't have a state to monitor
				}
				objName := seriesInfoMap[objType]
				if !objName.Exists() {
					lg.Warnf("Failed to find object name for series %s", seriesInfo)
					continue
				}
				if _, ok := resKubeState.Clusters[cl.Id]; !ok {
					resKubeState.Clusters[cl.Id] = &alertingv1.KubeObjectGroups{
						ResourceTypes: map[string]*alertingv1.NamespaceObjects{},
					}
				}
				if _, ok := resKubeState.Clusters[cl.Id].
					ResourceTypes[objType]; !ok {
					resKubeState.Clusters[cl.Id].
						ResourceTypes[objType] = &alertingv1.NamespaceObjects{
						Namespaces: map[string]*alertingv1.ObjectList{},
					}
				}
				if _, ok := resKubeState.Clusters[cl.Id].
					ResourceTypes[objType].
					Namespaces[namespace]; !ok {
					resKubeState.Clusters[cl.Id].
						ResourceTypes[objType].
						Namespaces[namespace] = &alertingv1.ObjectList{
						Objects: []string{},
					}
				}
				resKubeState.Clusters[cl.Id].
					ResourceTypes[objType].
					Namespaces[namespace].Objects = append(
					resKubeState.Clusters[cl.Id].ResourceTypes[objType].Namespaces[namespace].Objects, objName.String())
			}
		}
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_KubeState{
			KubeState: resKubeState,
		},
	}, nil
}
