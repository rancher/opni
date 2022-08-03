/* API implementation
 */
package slo

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/storage"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func list[T proto.Message](ctx context.Context, kvc storage.KeyValueStoreT[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(ctx, prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}

func checkDatasource(datasource string) error {
	if _, ok := datasourceToSLO[datasource]; !ok {
		return shared.ErrInvalidDatasource
	}
	if _, ok := datasourceToService[datasource]; !ok {
		return shared.ErrInvalidDatasource
	}
	return nil
}

func (p *Plugin) GetSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOData, error) {
	return p.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
}

func (p *Plugin) ListSLOs(ctx context.Context, _ *emptypb.Empty) (*sloapi.ServiceLevelObjectiveList, error) {
	items, err := list(ctx, p.storage.Get().SLOs, "/slos")
	if err != nil {
		return nil, err
	}
	return &sloapi.ServiceLevelObjectiveList{
		Items: items,
	}, nil
}

func (p *Plugin) CreateSLO(ctx context.Context, slorequest *sloapi.CreateSLORequest) (*corev1.ReferenceList, error) {
	lg := p.logger

	if err := checkDatasource(slorequest.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	if err := ValidateInput(slorequest); err != nil {
		return nil, err
	}
	// fetch from service discovery backend
	metricName := slorequest.SLO.GetMetricName()
	svcInfoList := make([]*sloapi.ServiceInfo, len(slorequest.Services))
	for i, svc := range slorequest.Services {
		res, err := p.GetMetricId(ctx, &sloapi.MetricRequest{
			Name:       metricName,
			Datasource: slorequest.SLO.GetDatasource(),
			ServiceId:  svc.GetJobId(),
			ClusterId:  svc.GetClusterId(),
		})
		if err != nil {
			return nil, err
		}
		svcInfoList[i] = res
	}

	osloSpecs, err := ParseToOpenSLO(slorequest, svcInfoList, ctx, p.logger)
	if err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Number of generated OpenSLO specs from create SLO request : %d", len(osloSpecs)))
	sloStore := datasourceToSLO[slorequest.SLO.GetDatasource()].WithCurrentRequest(slorequest, ctx)
	return sloStore.Create(osloSpecs)
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.SLOData) (*emptypb.Empty, error) {
	lg := p.logger
	overrideLabels := req.SLO.GetLabels()
	existing, err := p.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}

	metricName := existing.SLO.GetMetricName()

	// fetch dynamically from service discovery backend
	res, err := p.GetMetricId(ctx, &sloapi.MetricRequest{
		Name:       metricName,
		Datasource: existing.SLO.GetDatasource(),
		ServiceId:  existing.Service.GetJobId(),
		ClusterId:  existing.Service.GetClusterId(),
	})
	if err != nil {
		return nil, err
	}

	osloSpecs, err := ParseToOpenSLO(&sloapi.CreateSLORequest{
		SLO:      req.SLO,
		Services: []*sloapi.Service{req.Service},
	}, []*sloapi.ServiceInfo{res}, ctx, lg)
	if err != nil {
		return nil, err
	}
	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
	newReq, err := sloStore.Update(osloSpecs, existing)
	if err != nil { // exit when update fails
		return nil, err
	}

	// Merge when everything else is done
	proto.Merge(existing, newReq)
	existing.SLO.Labels = overrideLabels
	if err := p.storage.Get().SLOs.Put(ctx, path.Join("/slos", req.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteSLO(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.logger
	existing, err := p.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.Id))
	if err != nil {
		lg.With("delete slo", req.Id).Error("failed to get slo to delete in K,V store")
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
	err = sloStore.Delete(existing)
	if err != nil {
		return nil, err
	}
	if err := p.storage.Get().SLOs.Delete(ctx, path.Join("/slos", req.Id)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) CloneSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOData, error) {
	existing, err := p.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	var anyError error
	clone := proto.Clone(existing).(*sloapi.SLOData)
	clone.Id = ""
	clone.SLO.Name = clone.SLO.Name + " - Copy"

	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(ref, ctx)

	newId, anyError := sloStore.Clone(clone)
	if newId == "" { // hit an error applying the SLO, so we need to exit before updating the K,V
		return nil, anyError
	}
	if err := p.storage.Get().SLOs.Put(ctx, path.Join("/slos", newId), clone); err != nil {
		return nil, err
	}

	return clone, anyError // can partially succeed even on error
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	existing, err := p.storage.Get().SLOs.Get(ctx, path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}

	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(ref, ctx)
	state, err := sloStore.Status(existing)
	if err != nil {
		return &sloapi.SLOStatus{
			State: sloapi.SLOStatusState_InternalError,
		}, nil
	}
	return state, nil
}

// -------- Service Discovery ---------

func (p *Plugin) ListServices(ctx context.Context, req *sloapi.ListServiceRequest) (*sloapi.ServiceList, error) {
	res := &sloapi.ServiceList{}
	lg := p.logger
	err := checkDatasource(req.Datasource)
	if err != nil {
		return nil, shared.ErrInvalidDatasource
	}

	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to list clusters: %v", err))
		return nil, err
	}
	if len(clusters.Items) == 0 {
		lg.Debug("Found no downstream clusters")
		return res, nil
	}
	backend := datasourceToService[req.Datasource]
	listCurBackend := backend.WithCurrentRequest(req, ctx)
	datasourceServices, err := listCurBackend.List(clusters)
	if err != nil {
		return nil, err
	}
	res.Items = append(res.Items, datasourceServices.Items...)
	return res, nil
}

// Assign a Job Id to a pre configured metric based on the service selected
func (p *Plugin) GetMetricId(ctx context.Context, metricRequest *sloapi.MetricRequest) (*sloapi.ServiceInfo, error) {

	if _, ok := query.AvailableQueries[metricRequest.Name]; !ok {
		return nil, shared.ErrInvalidMetric
	}

	datasource := metricRequest.GetDatasource()
	if err := checkDatasource(datasource); err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	serviceBackend := datasourceToService[datasource].WithCurrentRequest(metricRequest, ctx)
	metricIds, err := serviceBackend.GetMetricId()

	if err != nil {
		return nil, err
	}

	return &sloapi.ServiceInfo{
		MetricName:    metricRequest.Name,
		ClusterId:     metricRequest.ClusterId,
		JobId:         metricRequest.ServiceId,
		MetricIdGood:  metricIds.Good,
		MetricIdTotal: metricIds.Total,
	}, nil
}

func (p *Plugin) ListMetrics(ctx context.Context, services *sloapi.ServiceList) (*sloapi.MetricList, error) {
	lg := p.logger
	candidateMetrics, err := list(ctx, p.storage.Get().Metrics, "/metrics")
	if err != nil {
		return nil, err
	}
	if len(services.Items) == 0 {
		return &sloapi.MetricList{Items: candidateMetrics}, nil
	}

	sharedItems := []*sloapi.Metric{}
	for _, c := range candidateMetrics {
		sharedItems = append(sharedItems, proto.Clone(c).(*sloapi.Metric))
	}

	lock := make(chan struct{}, 1)
	timeoutDuration := 5 * time.Second
	var wgSVC sync.WaitGroup
	wgSVC.Add(len(services.Items))

	for _, svc := range services.Items {
		// check each service in the list
		go func(svc *sloapi.Service) {
			defer wgSVC.Done()

			// check each metric
			itemsToReconcile := Filter(ctx, svc, candidateMetrics)

			select {
			case lock <- struct{}{}: //need to lock as we reconcile
				sharedItems = reconcileSetOfMetrics(sharedItems, itemsToReconcile)
				lg.Debug(fmt.Sprintf("%v", sharedItems))
				<-lock
			case <-time.After(timeoutDuration):
				//
			}
		}(svc)
	}
	wgSVC.Wait()

	return &sloapi.MetricList{
		Items: sharedItems,
	}, nil
}
