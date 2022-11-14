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
	"golang.org/x/exp/slices"
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

func clusterHasNodeExporterMetrics(ctx context.Context, adminClient cortexadmin.CortexAdminClient, cl *corev1.Cluster) bool {
	q, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{cl.Id},
		Query:   "node_os_info",
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

			if clusterHasKubeStateMetrics(ctx, adminClient, cl) {
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
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
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
			// clusterIds
			temp := map[string]*alertingv1.CpuNodeGroup{}
			tempCpuStates := map[string]struct{}{}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warnf("failed to find node name for series %s", seriesInfo)
					nodeName = metrics.UnlabelledNode
				} else {
					nodeName = node.String()
				}
				if _, ok := temp[cl.Id]; !ok {
					temp[cl.Id] = &alertingv1.CpuNodeGroup{
						Nodes: map[string]*alertingv1.CpuInfo{
							nodeName: {},
						},
					}
				}
				// lock.Lock()

				// if curNodes, ok := resCpuState.Clusters[cl.Id]; !ok {
				// 	resCpuState.Clusters[cl.Id] = &alertingv1.CpuNodeGroup{
				// 		Nodes: map[string]*alertingv1.CpuInfo{
				// 			nodeName: {
				// 				CoreIds: []int64{},
				// 			},
				// 		},
				// 	}
				// } else {
				// 	if _, ok := curNodes.Nodes[nodeName]; !ok {
				// 		curNodes.Nodes[nodeName] = &alertingv1.CpuInfo{
				// 			CoreIds: []int64{},
				// 		}
				// 	}
				// }
				// lock.Unlock()
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
				// lock.Lock()
				tempCpuStates[cpuState.String()] = struct{}{}
				// lock.Unlock()
			}
			for node, validCore := range nodesToCores {
				if _, ok := temp[cl.Id].Nodes[node]; !ok {
					temp[cl.Id].Nodes[node] = &alertingv1.CpuInfo{
						CoreIds: []int64{},
					}
				}
				for core := range validCore {
					temp[cl.Id].Nodes[node].CoreIds = append(
						temp[cl.Id].Nodes[node].CoreIds,
						core,
					)
				}
			}
			lock.Lock()
			for cpuState := range tempCpuStates {
				if _, ok := cpuStates[cpuState]; !ok {
					cpuStates[cpuState] = struct{}{}
				}
			}
			if _, ok := resCpuState.Clusters[cl.Id]; !ok {
				resCpuState.Clusters[cl.Id] = temp[cl.Id]
			}
			lock.Unlock()
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
		UsageTypes: []string{},
		Clusters:   make(map[string]*alertingv1.MemoryNodeGroup),
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
	memUsage := make(map[string]struct{})

	var wg sync.WaitGroup
	var lock sync.Mutex

	for _, cl := range clusters.Items {
		wg.Add(1)
		cl := cl
		go func() {
			defer wg.Done()
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
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
			nodeToDevices := make(map[string]map[string]struct{})
			temp := make(map[string]*alertingv1.MemoryNodeGroup)
			tempMemUsage := make(map[string]struct{})
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				name := seriesInfoMap["__name__"].String()

				objType := metrics.MemoryUsageTypeExtractor.FindStringSubmatch(name)[1]
				if slices.Contains(metrics.MemoryMatcherRegexFilter, objType) {
					lock.Lock()
					tempMemUsage[objType] = struct{}{}
					lock.Unlock()
				}
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warnf("failed to find node name for series %s", seriesInfo)
					nodeName = metrics.UnlabelledNode
				} else {
					nodeName = node.String()
				}
				if _, ok := temp[cl.Id]; !ok {
					temp[cl.Id] = &alertingv1.MemoryNodeGroup{
						Nodes: map[string]*alertingv1.MemoryInfo{},
					}
				}
				temp[cl.Id].Nodes[nodeName] = &alertingv1.MemoryInfo{
					Devices: []string{},
				}
				device := seriesInfoMap[metrics.MemoryDeviceFilter]

				if device.Exists() {
					if _, ok := nodeToDevices[nodeName]; !ok {
						nodeToDevices[nodeName] = map[string]struct{}{}
					}
					nodeToDevices[nodeName][device.String()] = struct{}{}
				}
			}
			for nodeName, devices := range nodeToDevices {
				for device := range devices {
					if _, ok := temp[cl.Id]; !ok {
						temp[cl.Id] = &alertingv1.MemoryNodeGroup{
							Nodes: map[string]*alertingv1.MemoryInfo{},
						}
					}
					if _, ok := temp[cl.Id].Nodes[nodeName]; !ok {
						temp[cl.Id].Nodes[nodeName] = &alertingv1.MemoryInfo{
							Devices: []string{},
						}
					}
					temp[cl.Id].Nodes[nodeName].Devices = append(
						temp[cl.Id].Nodes[nodeName].Devices,
						device,
					)
				}
			}
			lock.Lock()
			for memType := range tempMemUsage {
				memUsage[memType] = struct{}{}
			}
			resMemoryChoices.Clusters[cl.Id] = temp[cl.Id]
			lock.Unlock()
		}()
	}
	wg.Wait()
	for usageType := range memUsage {
		resMemoryChoices.UsageTypes = append(resMemoryChoices.UsageTypes, usageType)
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Memory{
			Memory: resMemoryChoices,
		},
	}, nil
}

func (p *Plugin) fetchFsSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.Logger.With("handler", "fetchMemorySaturationInfo")
	resFilesystemChoices := &alertingv1.ListAlertConditionFilesystemSaturation{
		Clusters: make(map[string]*alertingv1.FilesystemNodeGroup),
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
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
				return
			}
			rawFsSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.FilesystemMatcherName,
			})
			if err != nil {
				lg.Warnf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err)
				return
			}
			result := gjson.Get(string(rawFsSeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warnf("no node fs metrics found for cluster %s : %s", cl.Id, result)
				return
			}
			nodeToDevice := make(map[string]map[string]struct{})
			nodeToMountpoints := make(map[string]map[string]struct{})
			temp := make(map[string]*alertingv1.FilesystemNodeGroup)
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
				if _, ok := temp[cl.Id]; !ok {
					temp[cl.Id] = &alertingv1.FilesystemNodeGroup{
						Nodes: map[string]*alertingv1.FilesystemInfo{
							nodeName: {
								Mountpoints: []string{},
								Devices:     []string{},
							},
						},
					}
				}
				device := seriesInfoMap[metrics.NodeExportFilesystemDeviceLabel]
				if device.Exists() {
					if _, ok := nodeToDevice[nodeName]; !ok {
						nodeToDevice[nodeName] = map[string]struct{}{}
					}
					nodeToDevice[nodeName][device.String()] = struct{}{}
				}
				mountpoint := seriesInfoMap[metrics.NodeExporterMountpointLabel]
				if mountpoint.Exists() {
					if _, ok := nodeToMountpoints[nodeName]; !ok {
						nodeToMountpoints[nodeName] = map[string]struct{}{}
					}
					nodeToMountpoints[nodeName][mountpoint.String()] = struct{}{}
				}
			}
			for nodeName, devices := range nodeToDevice {
				for device := range devices {
					if _, ok := temp[cl.Id].Nodes[nodeName]; !ok {
						temp[cl.Id].Nodes[nodeName] = &alertingv1.FilesystemInfo{
							Devices: []string{},
						}
					}

					temp[cl.Id].Nodes[nodeName].Devices = append(
						temp[cl.Id].Nodes[nodeName].Devices,
						device,
					)
				}
			}
			for nodeName, mountpoints := range nodeToMountpoints {
				for mountpoint := range mountpoints {
					temp[cl.Id].Nodes[nodeName].Mountpoints = append(
						temp[cl.Id].Nodes[nodeName].Mountpoints,
						mountpoint,
					)
				}
			}
			lock.Lock()
			resFilesystemChoices.Clusters[cl.Id] = temp[cl.Id]
			lock.Unlock()
		}()
	}
	wg.Wait()
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Fs{
			Fs: resFilesystemChoices,
		},
	}, nil
}
