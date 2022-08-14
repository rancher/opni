/* API implementation
 */
package slo

import (
	"context"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/storage"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"path"
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
	return nil, shared.ErrNotImplemented
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

func (p *Plugin) CreateSLO(ctx context.Context, slorequest *sloapi.CreateSLORequest) (*corev1.Reference, error) {
	//lg := p.logger
	if err := slorequest.Validate(); err != nil {
		return nil, err
	}
	reqSLO := slorequest.GetSlo()
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}

	s := NewSLO(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		LabelPairs{}, //FIXME
		LabelPairs{}, //FIXME
	)
	sloStore := datasourceToSLO[slorequest.GetSlo().GetDatasource()].WithCurrentRequest(slorequest, ctx)
	err := sloStore.Create(s)
	if err != nil {
		return nil, err
	}
	sloData := &sloapi.SLOData{
		Id:  s.GetId(),
		SLO: reqSLO,
	}
	if err := p.storage.Get().SLOs.Put(ctx, path.Join("/slos", s.GetId()), sloData); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: s.GetId()}, nil
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.SLOData) (*emptypb.Empty, error) {
	//lg := p.logger
	if err := req.Validate(); err != nil {
		return nil, err
	}
	existing, err := p.storage.Get().SLOs.Get(ctx, path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}
	reqSLO := req.GetSLO()
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}
	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
	newSLO := SLOFromId(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		LabelPairs{}, //FIXME
		LabelPairs{}, //FIXME
		req.Id,
	)
	err = sloStore.Update(newSLO, existing)
	if err != nil { // exit when update fails
		return nil, err
	}
	updatedSLO := &sloapi.SLOData{}
	// Merge when everything else is done
	if err := p.storage.Get().SLOs.Put(ctx, path.Join("/slos", req.Id), updatedSLO); err != nil {
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
	if err := existing.Validate(); err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	sloStore := datasourceToSLO[existing.SLO.GetDatasource()].WithCurrentRequest(ref, ctx)
	newId, newData, anyError := sloStore.Clone(existing)
	if anyError != nil {
		return nil, anyError
	}
	if err := p.storage.Get().SLOs.Put(ctx, path.Join("/slos", newId.Id), newData); err != nil {
		return nil, err
	}
	return newData, nil
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	return nil, shared.ErrNotImplemented
}

func (p *Plugin) Preview(ctx context.Context, req *sloapi.CreateSLORequest) (*emptypb.Empty, error) {
	return nil, shared.ErrNotImplemented
}

// -------- Service Discovery ---------

func (p *Plugin) ListServices(ctx context.Context, req *sloapi.ListServicesRequest) (*sloapi.ServiceList, error) {
	//lg := p.logger
	err := checkDatasource(req.Datasource)
	if err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	backend := datasourceToService[req.Datasource].WithCurrentRequest(req, ctx)
	return backend.ListServices()
}

func (p *Plugin) ListMetrics(ctx context.Context, req *sloapi.ListMetricsRequest) (*sloapi.MetricList, error) {
	err := checkDatasource(req.Datasource)
	if err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	backend := datasourceToService[req.Datasource].WithCurrentRequest(req, ctx)
	return backend.ListMetrics()
}

func (p *Plugin) ListEvents(ctx context.Context, req *sloapi.ListEventsRequest) (*sloapi.EventList, error) {
	// fetch labels & their label values for the given cluster & service
	if err := req.Validate(); err != nil {
		return nil, err
	}
	datasource := req.GetDatasource()
	if err := checkDatasource(datasource); err != nil {
		return nil, shared.ErrInvalidDatasource
	}
	backend := datasourceToService[datasource].WithCurrentRequest(req, ctx)
	return backend.ListEvents()
}
