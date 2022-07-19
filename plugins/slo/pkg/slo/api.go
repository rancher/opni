/* API implementation
 */
package slo

import (
	"context"
	"fmt"
	"path"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func list[T proto.Message](kvc system.KVStoreClient[T], prefix string) ([]T, error) {
	keys, err := kvc.ListKeys(prefix)
	if err != nil {
		return nil, err
	}
	items := make([]T, len(keys))
	for i, key := range keys {
		item, err := kvc.Get(key)
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
	return p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
}

func (p *Plugin) ListSLOs(ctx context.Context, _ *emptypb.Empty) (*sloapi.ServiceLevelObjectiveList, error) {
	items, err := list(p.storage.Get().SLOs, "/slos")
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
	osloSpecs, err := ParseToOpenSLO(slorequest, ctx, p.logger)
	if err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Number of generated OpenSLO specs from create SLO request : %d", len(osloSpecs)))
	sloStore := datasourceToSLO[slorequest.SLO.GetDatasource()].WithCurrentRequest(slorequest, ctx)
	return sloStore.Create(osloSpecs)
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.SLOData) (*emptypb.Empty, error) {
	lg := p.logger
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	osloSpecs, err := ParseToOpenSLO(&sloapi.CreateSLORequest{
		SLO:      req.SLO,
		Services: []*sloapi.Service{req.Service},
	}, ctx, lg)
	if err != nil {
		return nil, err
	}
	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
	newReq, anyError := sloStore.Update(osloSpecs, existing)

	// Merge when everything else is done
	proto.Merge(existing, newReq)
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", req.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, anyError
}

func (p *Plugin) DeleteSLO(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
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
	if err := p.storage.Get().SLOs.Delete(path.Join("/slos", req.Id)); err != nil {
		return nil, err
	}
	// delete if found
	p.storage.Get().SLOs.Delete(path.Join("/slo_state", req.Id))
	return &emptypb.Empty{}, nil
}

func (p *Plugin) CloneSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOData, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
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
	newId, err := sloStore.Clone(clone)
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", newId), clone); err != nil {
		return nil, err
	}

	return clone, anyError
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
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

func (p *Plugin) ListServices(ctx context.Context, req *emptypb.Empty) (*sloapi.ServiceList, error) {
	res := &sloapi.ServiceList{}
	lg := p.logger
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to list clusters: %v", err))
		return nil, err
	}
	if len(clusters.Items) == 0 {
		lg.Debug("Found no downstream clusters")
		return res, nil
	}
	var anyError error
	for name, backend := range datasourceToService {
		lg.Debug("Found available datasource for service discovery : ", name)
		listCurBackend := backend.WithCurrentRequest(req, ctx)
		datasourceServices, err := listCurBackend.List(clusters)
		if err != nil {
			anyError = err
		}
		res.Items = append(res.Items, datasourceServices.Items...)
	}

	return res, anyError
}

// Assign a Job Id to a pre configured metric based on the service selected
func (p *Plugin) GetMetricId(ctx context.Context, metricRequest *sloapi.MetricRequest) (*sloapi.Service, error) {
	lg := p.logger

	if _, ok := query.AvailableQueries[metricRequest.Name]; !ok {
		return nil, shared.ErrInvalidMetric
	}

	lg.Debug(fmt.Sprintf("%v", datasourceToService))
	if _, ok := datasourceToService[metricRequest.Datasource]; !ok {
		return nil, shared.ErrInvalidDatasource
	}
	serviceBackend := datasourceToService[metricRequest.GetDatasource()].WithCurrentRequest(metricRequest, ctx)
	metricIds, err := serviceBackend.GetMetricId()

	if err != nil {
		return nil, err
	}

	return &sloapi.Service{
		MetricName:    metricRequest.Name,
		ClusterId:     metricRequest.ClusterId,
		JobId:         metricRequest.ServiceId,
		MetricIdGood:  metricIds.Good,
		MetricIdTotal: metricIds.Total,
	}, nil
}

func (p *Plugin) ListMetrics(ctx context.Context, _ *emptypb.Empty) (*sloapi.MetricList, error) {
	items, err := list(p.storage.Get().Metrics, "/metrics")
	if err != nil {
		return nil, err
	}
	return &sloapi.MetricList{
		Items: items,
	}, nil
}
