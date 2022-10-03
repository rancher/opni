package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/tidwall/gjson"
)

func handleChoicesByType(
	p *Plugin,
	ctx context.Context,
	req *alertingv1alpha.AlertDetailChoicesRequest,
) (*alertingv1alpha.ListAlertTypeDetails, error) {
	if req.GetAlertType() == alertingv1alpha.AlertType_SYSTEM {
		return nil, validation.Error("System alerts are not supported to be created via the dashboard")
	}
	if req.GetAlertType() == alertingv1alpha.AlertType_KUBE_STATE {
		return fetchKubeStateInfo(p, ctx)
	}
	return nil, shared.AlertingErrNotImplemented
}
func clusterHasKubeStateMetrics(adminClient cortexadmin.CortexAdminClient, cl *corev1.Cluster) bool {
	return true
}

func fetchKubeStateInfo(p *Plugin, ctx context.Context) (*alertingv1alpha.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchKubeStateInfo")
	resKubeState := &alertingv1alpha.ListAlertConditionKubeState{
		ClusterToObjects: map[string]*alertingv1alpha.KubeObjectGroups{},
		States:           metrics.KubeStates,
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
				if _, ok := resKubeState.ClusterToObjects[cl.Id]; !ok {
					resKubeState.ClusterToObjects[cl.Id] = &alertingv1alpha.KubeObjectGroups{
						ObjectTypeToNamespaces: map[string]*alertingv1alpha.NamespaceObjects{},
					}
				}
				if _, ok := resKubeState.ClusterToObjects[cl.Id].
					ObjectTypeToNamespaces[objType]; !ok {
					resKubeState.ClusterToObjects[cl.Id].
						ObjectTypeToNamespaces[objType] = &alertingv1alpha.NamespaceObjects{
						NamespaceToObjectList: map[string]*alertingv1alpha.ObjectList{},
					}
				}
				if _, ok := resKubeState.ClusterToObjects[cl.Id].
					ObjectTypeToNamespaces[objType].
					NamespaceToObjectList[namespace]; !ok {
					resKubeState.ClusterToObjects[cl.Id].
						ObjectTypeToNamespaces[objType].
						NamespaceToObjectList[namespace] = &alertingv1alpha.ObjectList{
						Objects: []string{},
					}
				}
				resKubeState.ClusterToObjects[cl.Id].
					ObjectTypeToNamespaces[objType].
					NamespaceToObjectList[namespace].Objects = append(
					resKubeState.ClusterToObjects[cl.Id].ObjectTypeToNamespaces[objType].NamespaceToObjectList[namespace].Objects, objName.String())
			}
		}
	}
	return &alertingv1alpha.ListAlertTypeDetails{
		Type: &alertingv1alpha.ListAlertTypeDetails_KubeState{
			KubeState: resKubeState,
		},
	}, nil
}
