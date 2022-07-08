/* API implementation
 */
package slo

import (
	"context"
	"fmt"
	"path"

	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
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
	if _, ok := datasourceToImpl[datasource]; !ok {
		return shared.ErrInvalidDatasource
	}
	return nil
}

func (p *Plugin) GetSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOImplData, error) {
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

func (p *Plugin) CreateSLO(ctx context.Context, slorequest *sloapi.CreateSLORequest) (*sloapi.CreatedSLOs, error) {
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
	sloStore := datasourceToImpl[slorequest.SLO.GetDatasource()].WithCurrentRequest(slorequest, ctx)
	return sloStore.Create(osloSpecs)
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.SLOImplData) (*emptypb.Empty, error) {
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
	sloStore := datasourceToImpl[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
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
	sloStore := datasourceToImpl[existing.SLO.GetDatasource()].WithCurrentRequest(req, ctx)
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

func (p *Plugin) CloneSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOImplData, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}
	if err := checkDatasource(existing.SLO.GetDatasource()); err != nil {
		return nil, err
	}
	var anyError error
	clone := proto.Clone(existing).(*sloapi.SLOImplData)
	clone.Id = ""
	clone.SLO.Name = clone.SLO.Name + " - Copy"

	sloStore := datasourceToImpl[existing.SLO.GetDatasource()].WithCurrentRequest(ref, ctx)
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

	sloStore := datasourceToImpl[existing.SLO.GetDatasource()].WithCurrentRequest(ref, ctx)
	state, err := sloStore.Status(existing)
	if err != nil {
		return &sloapi.SLOStatus{
			State: sloapi.SLOStatusState_SLO_STATUS_ERROR,
		}, nil
	}
	return state, nil
}

// -------- Service Discovery ---------

func (p *Plugin) GetService(ctx context.Context, ref *corev1.Reference) (*sloapi.Service, error) {
	return p.storage.Get().Services.Get(path.Join("/services", ref.Id))
}

func (p *Plugin) ListServices(ctx context.Context, _ *emptypb.Empty) (*sloapi.ServiceList, error) {
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
	cl := make([]string, 0)
	for _, c := range clusters.Items {
		cl = append(cl, c.Id)
		lg.Debug("Found cluster with id %v", c.Id)
	}
	discoveryQuery := `group by(job)({__name__!=""})`

	for _, c := range clusters.Items {
		resp, err := p.adminClient.Get().Query(ctx, &cortexadmin.QueryRequest{
			Tenants: []string{c.Id},
			Query:   discoveryQuery,
		})
		if err != nil {
			lg.Error(fmt.Sprintf("Failed to query cluster %v: %v", c.Id, err))
			return nil, err
		}
		data := resp.GetData()
		lg.Debug(fmt.Sprintf("Received service data:\n %s from cluster %s ", string(data), c.Id))
		q, err := unmarshal.UnmarshallPrometheusResponse(data)
		switch q.V.Type() {
		case model.ValVector:
			{
				vv := q.V.(model.Vector)
				for _, v := range vv {

					res.Items = append(res.Items, &sloapi.Service{
						JobId:     string(v.Metric["job"]),
						ClusterId: c.Id,
					})
				}
			}
		}
	}
	return res, nil
}

// Assign a Job Id to a pre configured metric based on the service selected
func (p *Plugin) GetMetricId(ctx context.Context, metricRequest *sloapi.MetricRequest) (*sloapi.Service, error) {
	lg := p.logger
	var goodMetricId string
	var totalMetricId string
	var err error

	if _, ok := query.AvailableQueries[metricRequest.Name]; !ok {
		return nil, shared.ErrInvalidMetric
	}
	switch metricRequest.Datasource {
	case shared.MonitoringDatasource:
		goodMetricId, totalMetricId, err = assignMetricToJobId(p, ctx, metricRequest)
		if err != nil {
			lg.Error(fmt.Sprintf("Unable to assign metric to job: %v", err))
			return nil, err
		}
	case shared.LoggingDatasource:
		return nil, shared.ErrNotImplemented
	}
	return &sloapi.Service{
		MetricName:    metricRequest.Name,
		ClusterId:     metricRequest.ClusterId,
		JobId:         metricRequest.ServiceId,
		MetricIdGood:  goodMetricId,
		MetricIdTotal: totalMetricId,
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
