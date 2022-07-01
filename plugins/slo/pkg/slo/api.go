/* API implementation
 */
package slo

import (
	"context"
	"fmt"
	"path"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (p *Plugin) CreateSLO(ctx context.Context, slo *sloapi.ServiceLevelObjective) (*emptypb.Empty, error) {
	if slo.Id == "" {
		slo.Id = uuid.New().String()
	} else {
		_, err := p.storage.Get().SLOs.Get(path.Join("/slos", slo.Id))
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return nil, err
			}
		} else {
			return nil, status.Error(codes.AlreadyExists, "SLO with this ID already exists")
		}
	}

	if err := ValidateInput(slo); err != nil {
		return nil, err
	}

	osloSpecs, err := ParseToOpenSLO(slo, ctx, p.logger)
	if err != nil {
		return nil, err
	}

	for _, spec := range osloSpecs {
		switch slo.GetDatasource() {
		case LoggingDatasource:
			return nil, ErrNotImplemented
		case MonitoringDatasource:
			// TODO forward to "sloth"-like prometheus parser
		default:
			return nil, status.Error(codes.FailedPrecondition, "Invalid datasource should have already been checked")
		}

		fmt.Printf("%v", spec) // FIXME: remove
	}

	// Put in k,v store only if everything else succeeds
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", slo.Id), slo); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.ServiceLevelObjective, error) {
	return p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
}

func (p *Plugin) ListSLOs(ctx context.Context, _ *emptypb.Empty) (*sloapi.ServiceLevelObjectiveList, error) {
	items, err := list(p.storage.Get().SLOs, "/slos")
	if err != nil {
		return nil, err
	}
	return &sloapi.ServiceLevelObjectiveList{
		Items: items,
	}, ErrNotImplemented
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.ServiceLevelObjective) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}

	proto.Merge(existing, req)
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", req.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, ErrNotImplemented
}

func (p *Plugin) DeleteSLO(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().SLOs.Delete(path.Join("/slos", ref.Id)); err != nil {
		return nil, err
	}

	p.storage.Get().SLOs.Delete(path.Join("/slo_state", ref.Id))
	return &emptypb.Empty{}, ErrNotImplemented
}

func (p *Plugin) CloneSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.ServiceLevelObjective, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}

	clone := proto.Clone(existing).(*sloapi.ServiceLevelObjective)
	clone.Id = ""
	clone.Name = clone.Name + " - Copy"
	if _, err := p.CreateSLO(ctx, clone); err != nil {
		return nil, err
	}

	return clone, ErrNotImplemented
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	return nil, ErrNotImplemented
}

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
func (p *Plugin) GetMetricId(ctx context.Context, metricRequest *sloapi.MetricRequest) (*sloapi.Metric, error) {
	lg := p.logger
	var metricId string
	var err error
	switch metricRequest.Datasource {
	case MonitoringDatasource:
		metricId, err = assignMetricToJobId(p, ctx, metricRequest)
		if err != nil {
			lg.Error(fmt.Sprintf("Unable to assign metric to job: %v", err))
			return nil, err
		}
	case LoggingDatasource:
		return nil, ErrNotImplemented
	}
	return &sloapi.Metric{
		Name:       metricRequest.Name,
		Datasource: metricRequest.Datasource,
		ClusterId:  metricRequest.ClusterId,
		ServiceId:  metricRequest.ServiceId,
		MetricId:   metricId,
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

func (p *Plugin) GetFormulas(ctx context.Context, ref *corev1.Reference) (*sloapi.Formula, error) {
	// return p.storage.Get().Formulas.Get(path.Join("/formulas", ref.Id))
	return nil, ErrNotImplemented
}

func (p *Plugin) ListFormulas(ctx context.Context, _ *emptypb.Empty) (*sloapi.FormulaList, error) {
	items, err := list(p.storage.Get().Formulas, "/formulas")
	if err != nil {
		return nil, err
	}
	return &sloapi.FormulaList{
		Items: items,
	}, ErrNotImplemented
}

func (p *Plugin) SetState(ctx context.Context, req *sloapi.SetStateRequest) (*emptypb.Empty, error) {
	// if err := p.storage.Get().SLOState.Put(path.Join("/slo_state", req.Slo.Id), req.State); err != nil {
	// 	return nil, err
	// }
	return &emptypb.Empty{}, ErrNotImplemented

}

func (p *Plugin) GetState(ctx context.Context, ref *corev1.Reference) (*sloapi.State, error) {
	// return p.storage.Get().SLOState.Get(path.Join("/slo_state", ref.Id))
	return nil, ErrNotImplemented
}
