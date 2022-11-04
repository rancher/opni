package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func handleChoicesByType(
	p *Plugin,
	ctx context.Context,
	req *alertingv1.AlertDetailChoicesRequest,
) (*alertingv1.ListAlertTypeDetails, error) {
	switch req.GetAlertType() {
	case alertingv1.AlertType_SYSTEM:
		return p.fetchAgentInfo(ctx)
	case alertingv1.AlertType_KUBE_STATE:
		return p.fetchKubeStateInfo(ctx)
	case alertingv1.AlertType_CPU_SATURATION:
		return p.fetchCPUSaturationInfo(ctx)
	case alertingv1.AlertType_MEMORY_SATURATION:
		return p.fetchMemorySaturationInfo(ctx)
	case alertingv1.AlertType_DISK_SATURATION:
		return p.fetchDiskSaturationInfo(ctx)
	case alertingv1.AlertType_FS_SATURATION:
		return p.fetchFsSaturationInfo(ctx)
	default:
		return nil, shared.AlertingErrNotImplemented
	}
}
func clusterHasKubeStateMetrics(ctx context.Context, adminClient cortexadmin.CortexAdminClient, cl *corev1.Cluster) bool {
	q, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{cl.Id},
		Query:   "kube_namespace_created{namespace=\"kube-system\"}",
	})
	if err != nil {
		return false
	}
	qr, err := unmarshal.UnmarshalPrometheusResponse(q.Data)
	if err != nil {
		return false
	}
	v, err := qr.GetVector()
	if err != nil {
		return false
	}
	if v == nil {
		return false
	}
	return len(*v) > 0
}

func (p *Plugin) fetchAgentInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
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

func (p *Plugin) fetchKubeStateInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchKubeStateInfo")
	resKubeState := &alertingv1.ListAlertConditionKubeState{
		Clusters: map[string]*alertingv1.KubeObjectGroups{},
		States:   shared.KubeStates,
	}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("failed to get cortex client %s", err)
	}
	var wg sync.WaitGroup
	var lock sync.Mutex
	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()

			if clusterHasKubeStateMetrics(adminClient, cl) {
				rawKubeStateSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
					Tenant:    cl.Id,
					MatchExpr: metrics.KubeObjMetricNameMatcher,
				})
				if err != nil {
					lg.Warnf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.KubeObjMetricNameMatcher, err)
					return
				}
				result := gjson.Get(string(rawKubeStateSeries.Data), "data.result")
				if !result.Exists() || len(result.Array()) == 0 {
					lg.Warnf("no result kube state metrics found for cluster %s : %s", cl.Id, result)
					return
				}
				for _, series := range result.Array() {
					seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
					// value array is [timestamp int, value int]
					seriesTsAndValue := series.Get("value")

					if !seriesInfo.Exists() || !seriesTsAndValue.Exists() {
						lg.Warnf("series info or value does not exist %s", series)
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
						lg.Warnf("failed to find object name for series %s", seriesInfo)
						continue
					}
					lock.Lock()
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
					lock.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_KubeState{
			KubeState: resKubeState,
		},
	}, nil
}

func (p *Plugin) fetchCPUSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchCPUSaturationInfo")
	resCpuState := &alertingv1.ListAlertConditionCPUSaturation{
		Clusters:  make(map[string]*alertingv1.CpuNodeGroup),
		CpuStates: []string{},
	}

	cpuStates := make(map[string]struct{})
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error("failed to get clusters", zap.Error(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("failed to get cortex client %s", err)
	}

	var wg sync.WaitGroup
	var lock sync.Mutex

	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()
			if !clusterHasNodeExporterMetrics(adminClient, cl) {
				return
			}
			nodesToCores := map[string]map[int64]struct{}{}
			rawCPUSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.CPUMatcherName,
			})
			if err != nil {
				lg.Warnf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err)
				return
			}
			result := gjson.Get(string(rawCPUSeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warnf("no result kube state metrics found for cluster %s : %s", cl.Id, result)
				return
			}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warnf("failed to find node name for series %s", seriesInfo)
					nodeName = metrics.UnlabelledNode
				}
				lock.Lock()
				nodeName = node.String()
				if curNodes, ok := resCpuState.Clusters[cl.Id]; !ok {
					resCpuState.Clusters[cl.Id] = &alertingv1.CpuNodeGroup{
						Nodes: map[string]*alertingv1.CpuInfo{
							nodeName: {
								CoreIds: []int64{},
							},
						},
					}
				} else {
					if _, ok := curNodes.Nodes[nodeName]; !ok {
						curNodes.Nodes[nodeName] = &alertingv1.CpuInfo{
							CoreIds: []int64{},
						}
					}
				}
				lock.Unlock()
				core := seriesInfoMap["cpu"]
				if !core.Exists() {
					lg.Warnf("failed to find core name for series %s", seriesInfo)
					continue
				}
				if _, ok := nodesToCores[nodeName]; !ok {
					nodesToCores[nodeName] = map[int64]struct{}{}
				}
				nodesToCores[nodeName][core.Int()] = struct{}{}

				cpuState := seriesInfoMap["mode"]
				if !cpuState.Exists() {
					lg.Warnf("failed to find cpu state for series %s", seriesInfo)
					continue
				}
				lock.Lock()
				cpuStates[cpuState.String()] = struct{}{}
				lock.Unlock()
			}
			for node, validCore := range nodesToCores {
				lock.Lock()
				for core := range validCore {
					resCpuState.Clusters[cl.Id].Nodes[node].CoreIds = append(
						resCpuState.Clusters[cl.Id].Nodes[node].CoreIds,
						core,
					)
				}
				lock.Unlock()
			}
		}()
	}
	wg.Wait()
	for cpuState := range cpuStates {
		resCpuState.CpuStates = append(resCpuState.CpuStates, cpuState)
	}

	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Cpu{
			Cpu: resCpuState,
		},
	}, nil
}

func (p *Plugin) fetchMemorySaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchMemorySaturationInfo")
	resMemoryChoices := &alertingv1.ListAlertConditionMemorySaturation{
		Clusters:   make(map[string]*alertingv1.MemoryNodeGroup),
		UsageTypes: []string{},
	}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error("failed to get clusters", zap.Error(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("failed to get cortex client %s", err)
	}

	var wg sync.WaitGroup
	var lock sync.Mutex

	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()
			if !clusterHasNodeExporterMetrics(adminClient, cl) {
				return
			}
			rawMemorySeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.MemoryMatcherName,
			})
			if err != nil {
				lg.Warnf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err)
				return
			}
			result := gjson.Get(string(rawMemorySeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warnf("no node memory metrics found for cluster %s : %s", cl.Id, result)
				return
			}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warnf("failed to find node name for series %s", seriesInfo)
					nodeName = metrics.UnlabelledNode
				}
				nodeName = node.String()
				lock.Lock()
				if _, ok := resMemoryChoices.Clusters[cl.Id]; !ok {
					resMemoryChoices.Clusters[cl.Id] = &alertingv1.MemoryNodeGroup{
						Nodes: map[string]*alertingv1.MemoryInfo{
							nodeName: {
								Device: []string{},
							},
						},
					}
				}
				lock.Unlock()
				name := seriesInfoMap["__name__"].String()
				usageType := metrics.MemoryUsageTypeExtractor.FindStringSubmatch(name)[1]
				lock.Lock()
				resMemoryChoices.UsageTypes = append(resMemoryChoices.UsageTypes, usageType)
				lock.Unlock()
				device := seriesInfoMap[metrics.MemoryDeviceFilter]
				if !device.Exists() {
					lg.Warnf("failed to find device name for series %s", seriesInfo)
					continue
				}
				lock.Lock()
				resMemoryChoices.Clusters[cl.Id].Nodes[nodeName].Device = append(
					resMemoryChoices.Clusters[cl.Id].Nodes[nodeName].Device,
					device.String(),
				)
				lock.Unlock()
			}
		}()
	}
	wg.Wait()
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Memory{
			Memory: resMemoryChoices,
		},
	}, nil
}

func (p *Plugin) fetchDiskSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchMemorySaturationInfo")
	resDiskChoices := &alertingv1.ListAlertConditionDiskSaturation{}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error("failed to get clusters", zap.Error(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("failed to get cortex client %s", err)
	}

	var wg sync.WaitGroup
	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()
			if !clusterHasNodeExporterMetrics(adminClient, cl) {
				return
			}
		}()
	}

	wg.Wait()
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Disk{
			Disk: resDiskChoices,
		},
	}, shared.AlertingErrNotImplemented
}

func (p *Plugin) fetchFsSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchMemorySaturationInfo")
	resDiskChoices := &alertingv1.ListAlertConditionFilesystemSaturation{}
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error("failed to get clusters", zap.Error(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Errorf("failed to get cortex client %s", err)
	}

	var wg sync.WaitGroup
	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()
			if !clusterHasNodeExporterMetrics(adminClient, cl) {
				return
			}
		}()
	}
	wg.Wait()
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Fs{
			Fs: resDiskChoices,
		},
	}, shared.AlertingErrNotImplemented
}
