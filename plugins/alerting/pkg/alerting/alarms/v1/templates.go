package alarms

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"slices"

	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/metrics/naming"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/types/known/emptypb"
)

func handleChoicesByType(
	ctx context.Context,
	p *AlarmServerComponent,
	req *alertingv1.AlertDetailChoicesRequest,
) (*alertingv1.ListAlertTypeDetails, error) {
	switch req.GetAlertType() {
	case alertingv1.AlertType_System:
		return p.fetchAgentInfo(ctx)
	case alertingv1.AlertType_DownstreamCapability:
		return p.fetchDownstreamCapabilityInfo(ctx)
	case alertingv1.AlertType_KubeState:
		return p.fetchKubeStateInfo(ctx)
	case alertingv1.AlertType_CpuSaturation:
		return p.fetchCPUSaturationInfo(ctx)
	case alertingv1.AlertType_MemorySaturation:
		return p.fetchMemorySaturationInfo(ctx)
	case alertingv1.AlertType_FsSaturation:
		return p.fetchFsSaturationInfo(ctx)
	case alertingv1.AlertType_PrometheusQuery:
		return p.fetchPrometheusQueryInfo(ctx)
	case alertingv1.AlertType_MonitoringBackend:
		return p.fetchMonitoringBackendInfo(ctx)
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
	qr, err := compat.UnmarshalPrometheusResponse(q.Data)
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
	qr, err := compat.UnmarshalPrometheusResponse(q.Data)
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

func (p *AlarmServerComponent) fetchAgentInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	ctxCa, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	mgmtClient, err := p.mgmtClient.GetContext(ctxCa)
	if err != nil {
		return nil, err
	}
	clusters, err := mgmtClient.ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
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

func (p *AlarmServerComponent) fetchDownstreamCapabilityInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	ctxCa, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	mgmtClient, err := p.mgmtClient.GetContext(ctxCa)
	if err != nil {
		return nil, err
	}
	clusters, err := mgmtClient.ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		return nil, err
	}
	resChoices := &alertingv1.ListAlertConditionDownstreamCapability{
		Clusters: make(map[string]*alertingv1.CapabilityState),
	}

	for _, cl := range clusters.Items {
		resChoices.Clusters[cl.Id] = &alertingv1.CapabilityState{
			States: []string{health.StatusPending.String(), health.StatusFailure.String()},
		}
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_DownstreamCapability{
			DownstreamCapability: resChoices,
		},
	}, nil
}

func (p *AlarmServerComponent) fetchKubeStateInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.logger.With("handler", "fetchKubeStateInfo")
	resKubeState := &alertingv1.ListAlertConditionKubeState{
		Clusters: map[string]*alertingv1.KubeObjectGroups{},
		States:   shared.KubeStates,
	}
	clusters, err := p.mgmtClient.Get().ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to get cortex client %s", err))
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
					MatchExpr: naming.KubeObjMetricNameMatcher,
				})
				if err != nil {
					lg.Warn(fmt.Sprintf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, naming.KubeObjMetricNameMatcher, err))
					return
				}
				result := gjson.Get(string(rawKubeStateSeries.Data), "data.result")
				if !result.Exists() || len(result.Array()) == 0 {
					lg.Warn(fmt.Sprintf("no result kube state metrics found for cluster %s : %s", cl.Id, result))
					return
				}
				for _, series := range result.Array() {
					seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
					// value array is [timestamp int, value int]
					seriesTsAndValue := series.Get("value")

					if !seriesInfo.Exists() || !seriesTsAndValue.Exists() {
						lg.Warn(fmt.Sprintf("series info or value does not exist %s", series))
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

					objType := naming.KubeObjTypeExtractor.FindStringSubmatch(name)[1]
					if objType == "namespace" {
						continue // namespaces don't have a state to monitor
					}
					objName := seriesInfoMap[objType]
					if !objName.Exists() {
						lg.Warn(fmt.Sprintf("failed to find object name for series %s", seriesInfo))
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

type clusterCpuSaturation struct {
	clusterId    string
	cpuStates    []string
	cpuNodeGroup *alertingv1.CpuNodeGroup
}

func (p *AlarmServerComponent) fetchCPUSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.logger.With("handler", "fetchCPUSaturationInfo")
	clusters, err := p.mgmtClient.Get().ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		lg.Error("failed to get clusters", logger.Err(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to get cortex client %s", err))
	}

	results := make(chan (<-chan *clusterCpuSaturation), len(clusters.Items))
	for _, cl := range clusters.Items {
		cl := cl
		results <- lo.Async(func() *clusterCpuSaturation {
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
				return &clusterCpuSaturation{
					clusterId: cl.Id,
				}
			}
			rawCPUSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.CPUMatcherName,
			})
			if err != nil {
				lg.Warn(fmt.Sprintf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err))
				return &clusterCpuSaturation{
					clusterId: cl.Id,
				}
			}
			result := gjson.Get(string(rawCPUSeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warn(fmt.Sprintf("no result kube state metrics found for cluster %s : %s", cl.Id, result))
				return &clusterCpuSaturation{
					clusterId: cl.Id,
				}
			}
			// clusterIds
			cpuNodeGroup := &alertingv1.CpuNodeGroup{
				Nodes: make(map[string]*alertingv1.CpuInfo),
			}
			tempCpuStates := map[string]struct{}{}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warn(fmt.Sprintf("failed to find node name for series %s", seriesInfo))
					nodeName = metrics.UnlabelledNode
				} else {
					nodeName = node.String()
				}
				if _, ok := cpuNodeGroup.Nodes[nodeName]; !ok {
					cpuNodeGroup.Nodes[nodeName] = &alertingv1.CpuInfo{
						CoreIds: []int64{},
					}
				}
				core := seriesInfoMap["cpu"]
				if !core.Exists() {
					lg.Warn(fmt.Sprintf("failed to find core name for series %s", seriesInfo))
					continue
				}
				if _, ok := cpuNodeGroup.Nodes[nodeName]; !ok {
					cpuNodeGroup.Nodes[nodeName] = &alertingv1.CpuInfo{
						CoreIds: []int64{},
					}
				}
				cpuNodeGroup.Nodes[nodeName].CoreIds = append(
					cpuNodeGroup.Nodes[nodeName].CoreIds, core.Int())

				cpuState := seriesInfoMap["mode"]
				if !cpuState.Exists() {
					lg.Warn(fmt.Sprintf("failed to find cpu state for series %s", seriesInfo))
					continue
				}
				tempCpuStates[cpuState.String()] = struct{}{}
			}
			return &clusterCpuSaturation{
				clusterId:    cl.Id,
				cpuStates:    lo.Keys(tempCpuStates),
				cpuNodeGroup: cpuNodeGroup,
			}
		})

	}
	close(results)
	usageTypes := make(map[string]struct{})
	clusterResults := make(map[string]*alertingv1.CpuNodeGroup)
	for result := range results {
		res := <-result
		if len(res.cpuStates) == 0 {
			continue
		}
		clusterResults[res.clusterId] = res.cpuNodeGroup
		for _, cpuState := range res.cpuStates {
			usageTypes[cpuState] = struct{}{}
		}
	}
	sorted := lo.Keys(usageTypes)
	sort.Strings(sorted)

	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Cpu{
			Cpu: &alertingv1.ListAlertConditionCPUSaturation{
				Clusters:  clusterResults,
				CpuStates: sorted,
			},
		},
	}, nil
}

type clusterMemorySaturation struct {
	id              string
	usageTypes      []string
	memoryNodeGroup *alertingv1.MemoryNodeGroup
}

func (p *AlarmServerComponent) fetchMemorySaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.logger.With("handler", "fetchMemorySaturationInfo")
	clusters, err := p.mgmtClient.Get().ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error("failed to get clusters", logger.Err(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to get cortex client %s", err))
	}

	results := make(chan (<-chan *clusterMemorySaturation), len(clusters.Items))
	for _, cl := range clusters.Items {
		cl := cl
		results <- lo.Async(func() *clusterMemorySaturation {
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
				return &clusterMemorySaturation{
					id: cl.Id,
				}
			}
			rawMemorySeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.MemoryMatcherName,
			})
			if err != nil {
				lg.Warn(fmt.Sprintf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err))
				return &clusterMemorySaturation{
					id: cl.Id,
				}
			}
			result := gjson.Get(string(rawMemorySeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warn(fmt.Sprintf("no node memory metrics found for cluster %s : %s", cl.Id, result))
				return &clusterMemorySaturation{
					id: cl.Id,
				}
			}
			nodeToDevices := make(map[string]map[string]struct{})
			memoryNodeGroup := &alertingv1.MemoryNodeGroup{
				Nodes: make(map[string]*alertingv1.MemoryInfo),
			}
			tempMemUsage := make(map[string]struct{})
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				name := seriesInfoMap["__name__"].String()

				objType := metrics.MemoryUsageTypeExtractor.FindStringSubmatch(name)[1]
				if slices.Contains(metrics.MemoryMatcherRegexFilter, objType) {
					tempMemUsage[objType] = struct{}{}
				}
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warn(fmt.Sprintf("failed to find node name for series %s", seriesInfo))
					nodeName = metrics.UnlabelledNode
				} else {
					nodeName = node.String()
				}
				memoryNodeGroup.Nodes[nodeName] = &alertingv1.MemoryInfo{
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
					if _, ok := memoryNodeGroup.Nodes[nodeName]; !ok {
						memoryNodeGroup.Nodes[nodeName] = &alertingv1.MemoryInfo{
							Devices: []string{},
						}
					}
					memoryNodeGroup.Nodes[nodeName].Devices = append(
						memoryNodeGroup.Nodes[nodeName].Devices,
						device,
					)
				}
			}
			return &clusterMemorySaturation{
				id:              cl.Id,
				usageTypes:      lo.Keys(tempMemUsage),
				memoryNodeGroup: memoryNodeGroup,
			}
		})
	}
	close(results)

	usageTypes := make(map[string]struct{})
	clusterResults := make(map[string]*alertingv1.MemoryNodeGroup)
	for result := range results {
		res := <-result
		if len(res.usageTypes) == 0 {
			continue
		}
		clusterResults[res.id] = res.memoryNodeGroup
		for _, usageType := range res.usageTypes {
			usageTypes[usageType] = struct{}{}
		}
	}

	sorted := lo.Keys(usageTypes)
	sort.Strings(sorted)
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Memory{
			Memory: &alertingv1.ListAlertConditionMemorySaturation{
				Clusters:   clusterResults,
				UsageTypes: sorted,
			},
		},
	}, nil
}

type clusterFilesystemSaturation struct {
	clusterId       string
	filesystemGroup *alertingv1.FilesystemNodeGroup
}

func (p *AlarmServerComponent) fetchFsSaturationInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	lg := p.logger.With("handler", "fetchMemorySaturationInfo")
	clusters, err := p.mgmtClient.Get().ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		lg.Error("failed to get clusters", logger.Err(err))
		return nil, err
	}
	adminClient, err := p.adminClient.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to get cortex client %s", err))
	}

	results := make(chan (<-chan *clusterFilesystemSaturation), len(clusters.Items))
	for _, cl := range clusters.Items {
		cl := cl
		results <- lo.Async(func() *clusterFilesystemSaturation {
			if !clusterHasNodeExporterMetrics(ctx, adminClient, cl) {
				return &clusterFilesystemSaturation{
					clusterId: cl.Id,
				}
			}
			rawFsSeries, err := adminClient.ExtractRawSeries(ctx, &cortexadmin.MatcherRequest{
				Tenant:    cl.Id,
				MatchExpr: metrics.FilesystemMatcherName,
			})
			if err != nil {
				lg.Warn(fmt.Sprintf("failed to extract raw series for cluster %s and match expr %s : %s", cl.Id, metrics.CPUMatcherName, err))
				return &clusterFilesystemSaturation{
					clusterId: cl.Id,
				}
			}
			result := gjson.Get(string(rawFsSeries.Data), "data.result")
			if !result.Exists() || len(result.Array()) == 0 {
				lg.Warn(fmt.Sprintf("no node fs metrics found for cluster %s : %s", cl.Id, result))
				return &clusterFilesystemSaturation{
					clusterId: cl.Id,
				}
			}
			nodeToDevice := make(map[string]map[string]struct{})
			nodeToMountpoints := make(map[string]map[string]struct{})

			filesystemNodeGroup := &alertingv1.FilesystemNodeGroup{
				Nodes: make(map[string]*alertingv1.FilesystemInfo),
			}
			for _, series := range result.Array() {
				seriesInfo := series.Get("metric") // this maps to {"metric" : {}, "value": []}
				seriesInfoMap := seriesInfo.Map()
				node := seriesInfoMap[metrics.NodeExporterNodeLabel]
				nodeName := ""
				if !node.Exists() {
					lg.Warn(fmt.Sprintf("failed to find node name for series %s", seriesInfo))
					nodeName = metrics.UnlabelledNode
				} else {
					nodeName = node.String()
				}
				if _, ok := filesystemNodeGroup.Nodes[nodeName]; !ok {
					filesystemNodeGroup.Nodes[nodeName] = &alertingv1.FilesystemInfo{
						Devices:     []string{},
						Mountpoints: []string{},
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
					if _, ok := filesystemNodeGroup.Nodes[nodeName]; !ok {
						filesystemNodeGroup.Nodes[nodeName] = &alertingv1.FilesystemInfo{
							Devices:     []string{},
							Mountpoints: []string{},
						}
					}
					filesystemNodeGroup.Nodes[nodeName].Devices = append(
						filesystemNodeGroup.Nodes[nodeName].Devices,
						device,
					)
				}
			}
			for nodeName, mountpoints := range nodeToMountpoints {
				for mountpoint := range mountpoints {
					if _, ok := filesystemNodeGroup.Nodes[nodeName]; !ok {
						filesystemNodeGroup.Nodes[nodeName] = &alertingv1.FilesystemInfo{
							Devices:     []string{},
							Mountpoints: []string{},
						}
					}
					filesystemNodeGroup.Nodes[nodeName].Mountpoints = append(
						filesystemNodeGroup.Nodes[nodeName].Mountpoints,
						mountpoint,
					)
				}
			}
			return &clusterFilesystemSaturation{
				clusterId:       cl.Id,
				filesystemGroup: filesystemNodeGroup,
			}

		})
	}
	close(results)
	clusterResults := make(map[string]*alertingv1.FilesystemNodeGroup)
	for result := range results {
		res := <-result
		if res == nil || res.filesystemGroup == nil {
			continue
		}
		if len(res.filesystemGroup.Nodes) == 0 {
			continue
		}
		clusterResults[res.clusterId] = res.filesystemGroup
	}

	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_Fs{
			Fs: &alertingv1.ListAlertConditionFilesystemSaturation{
				Clusters: clusterResults,
			},
		},
	}, nil
}

func (p *AlarmServerComponent) fetchPrometheusQueryInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	ctxca, ca := context.WithTimeout(ctx, time.Second*30)
	defer ca()

	mgmtClient, err := p.mgmtClient.GetContext(ctxca)
	if err != nil {
		return nil, err
	}
	cls, err := mgmtClient.ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		return nil, err
	}
	res := []string{}
	for _, cl := range cls.Items {
		for _, cap := range cl.GetCapabilities() {
			if cap.Name == wellknown.CapabilityMetrics {
				res = append(res, cl.Id)
				break
			}
		}
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_PrometheusQuery{
			PrometheusQuery: &alertingv1.ListAlertConditionPrometheusQuery{
				Clusters: res,
			},
		},
	}, nil

}

func (p *AlarmServerComponent) fetchMonitoringBackendInfo(ctx context.Context) (*alertingv1.ListAlertTypeDetails, error) {
	ctxca, ca := context.WithTimeout(ctx, time.Second*3)
	defer ca()
	cortexOps, err := p.cortexOpsClient.GetContext(ctxca)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire cortex ops client %s", err)
	}
	state, err := cortexOps.Status(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring backend status %s", err)
	}
	if state.InstallState == driverutil.InstallState_NotInstalled {
		return nil, validation.Error("monitoring backend is not installed")
	}
	return &alertingv1.ListAlertTypeDetails{
		Type: &alertingv1.ListAlertTypeDetails_MonitoringBackend{
			MonitoringBackend: &alertingv1.ListAlertConditionMonitoringBackend{
				BackendComponents: shared.CortexComponents,
			},
		},
	}, nil
}
