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

	// if err := ValidateInput(slo); err != nil {
	// 	return nil, err
	// }

	// osloSpecs, err := ParseToOpenSLO(slo, ctx, p.logger)
	// if err != nil {
	// 	return nil, err
	// }

	// for _, spec := range osloSpecs {
	// 	switch slo.GetDatasource() {
	// 	case LoggingDatasource:
	// 		// TODO translate OpenSLO to Transform OS api
	// 	case MonitoringDatasource:
	// 		// TODO forward to "sloth"-like prometheus parser
	// 	default:
	// 		return nil, status.Error(codes.FailedPrecondition, "Invalid datasource should have already been checked")
	// 	}

	// 	fmt.Printf("%v", spec) // FIXME: remove
	// }

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
	}, nil
}

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.ServiceLevelObjective) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}

	//TODO

	proto.Merge(existing, req)
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", req.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteSLO(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().SLOs.Delete(path.Join("/slos", ref.Id)); err != nil {
		return nil, err
	}

	//TODO remove prom rules / OS monitors based on config

	p.storage.Get().SLOs.Delete(path.Join("/slo_state", ref.Id))
	return &emptypb.Empty{}, nil
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
	return clone, nil
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	// TODO implement
	return &sloapi.SLOStatus{
		Slo:       ref,
		Timestamp: 0,
		Status:    nil,
		Budget:    nil,
	}, nil
}

func (p *Plugin) GetService(ctx context.Context, ref *corev1.Reference) (*sloapi.Service, error) {
	return p.storage.Get().Services.Get(path.Join("/services", ref.Id))
}

func (p *Plugin) ListServices(ctx context.Context, _ *emptypb.Empty) (*sloapi.ServiceList, error) {
	lg := p.logger
	clusters, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to list clusters: %v", err))
		return nil, err
	}
	if len(clusters.Items) == 0 {
		lg.Debug("Found no downstream clusters")
		return &sloapi.ServiceList{}, nil
	}
	cl := make([]string, 0)
	for _, c := range clusters.Items {
		cl = append(cl, c.Id)
	}

	discoveryQuery := `group by(job)({__name__!=""})`
	adminClient := cortexadmin.NewCortexAdminClient(managementv1.UnderlyingConn(p.mgmtClient.Get()))
	resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: cl,
		Query:   discoveryQuery,
	})

	if err != nil {
		lg.Error("Failed to get response for Query %s : %v", discoveryQuery, err)
	}

	data := resp.GetData()
	lg.Debug("Received service data:\n ", data)
	// Maybe TTL cache responses?
	var s model.Sample
	s.UnmarshalJSON(data)
	lg.Debug(fmt.Sprintf("%v", s))

	//TODO parse data into items

	// items, err := list(p.storage.Get().Services, "/services")
	// if err != nil {
	// 	return nil, err
	// }
	return &sloapi.ServiceList{}, nil
}

func (p *Plugin) GetMetric(ctx context.Context, ref *corev1.Reference) (*sloapi.Metric, error) {
	return p.storage.Get().Metrics.Get(path.Join("/metrics", ref.Id))
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
	return p.storage.Get().Formulas.Get(path.Join("/formulas", ref.Id))
}

func (p *Plugin) ListFormulas(ctx context.Context, _ *emptypb.Empty) (*sloapi.FormulaList, error) {
	items, err := list(p.storage.Get().Formulas, "/formulas")
	if err != nil {
		return nil, err
	}
	return &sloapi.FormulaList{
		Items: items,
	}, nil
}

func (p *Plugin) SetState(ctx context.Context, req *sloapi.SetStateRequest) (*emptypb.Empty, error) {
	if err := p.storage.Get().SLOState.Put(path.Join("/slo_state", req.Slo.Id), req.State); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetState(ctx context.Context, ref *corev1.Reference) (*sloapi.State, error) {
	return p.storage.Get().SLOState.Get(path.Join("/slo_state", ref.Id))
}
