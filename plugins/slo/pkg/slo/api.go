/* API implementation
 */
package slo

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"reflect"

	"github.com/google/uuid"
	json "github.com/json-iterator/go"
	pv1 "github.com/prometheus/client_golang/api/prometheus/v1"
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

// Struct for unmarshalling from github.com/prometheus/common/model
type queryResult struct {
	Type   model.ValueType `json:"resultType"`
	Result interface{}     `json:"result"`

	// The decoded value.
	v model.Value
}
type ErrorType string

// struct for unmarshalling from prometheus api responses
type apiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType ErrorType       `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// Unmarshalling for `queryResult`
func (qr *queryResult) UnmarshalJSON(b []byte) error {
	v := struct {
		Type   model.ValueType `json:"resultType"`
		Result json.RawMessage `json:"result"`
	}{}

	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	switch v.Type {
	case model.ValScalar:
		var sv model.Scalar
		err = json.Unmarshal(v.Result, &sv)
		qr.v = &sv

	case model.ValVector:
		var vv model.Vector
		err = json.Unmarshal(v.Result, &vv)
		qr.v = vv

	case model.ValMatrix:
		var mv model.Matrix
		err = json.Unmarshal(v.Result, &mv)
		qr.v = mv

	default:
		err = fmt.Errorf("unexpected value type %q", v.Type)
	}
	return err
}

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

	resp, err := p.adminClient.Get().Query(ctx, &cortexadmin.QueryRequest{
		Tenants: cl,
		Query:   discoveryQuery,
	})

	if err != nil {
		lg.Error("Failed to get response for Query %s : %v", discoveryQuery, err)
	}

	data := resp.GetData()
	lg.Debug(fmt.Sprintf("Received service data:\n %s ", string(data)))

	var a apiResponse
	var q queryResult
	var metric pv1.MetricMetadata
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Is nested data treated as nil? : %v ", a.Data == nil))
	lg.Debug(fmt.Sprintf("Unmarshalled service data:\n %v", a.Status))

	if err := json.Unmarshal(a.Data, &q); err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Unmarshalled query result type:\n %v", q.Type.String()))
	lg.Debug(fmt.Sprintf("Unmarshalled query result datatype:\n %v", reflect.TypeOf(q.Type)))
	if err := json.Unmarshal(a.Data, &metric); err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Unmarshalled metric metadata:\n %v", metric))
	//TODO parse data into items

	// items, err := list(p.storage.Get().Services, "/services")
	// if err != nil {
	// 	return nil, err
	// }
	return &sloapi.ServiceList{}, nil
}

func (p *Plugin) GetMetric(ctx context.Context, metricRequest *sloapi.MetricRequest) (*sloapi.Metric, error) {
	// validate we have the metrics persisted
	_, err := p.storage.Get().Metrics.Get(path.Join("/metrics/", metricRequest.Name, "/", metricRequest.ServiceId))
	if err != nil {
		return nil, err
	}
	var query bytes.Buffer
	if err = GetDownstreamMetricQueryTempl.Execute(&query, map[string]string{
		"serviceId": metricRequest.ServiceId,
		"Name":      metricRequest.Name,
	}); err != nil {
		return nil, err
	}
	//TODO(alex) : run the query against downstream cortex
	return nil, ErrNotImplemented
}

func (p *Plugin) ListMetrics(ctx context.Context, _ *emptypb.Empty) (*sloapi.MetricList, error) {
	if len(availableQueries) == 0 {
		InitMetricList()
		items := make([]sloapi.Metric, len(availableQueries))
		idx := 0
		for _, q := range availableQueries {
			items[idx] = sloapi.Metric{
				Name:              q.Name(),
				Datasource:        q.Datasource(),
				MetricDescription: q.Description(),
			}
			idx += 1
		}
		if err := p.storage.Get().Metrics.Put(path.Join("/metrics", items[0].Name), &items[0]); err != nil {
			return nil, err
		}
	}
	items, err := list(p.storage.Get().Metrics, "/metrics")
	if err != nil {
		return nil, err
	}
	return &sloapi.MetricList{
		Items: items,
	}, ErrNotImplemented
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
	if err := p.storage.Get().SLOState.Put(path.Join("/slo_state", req.Slo.Id), req.State); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, ErrNotImplemented

}

func (p *Plugin) GetState(ctx context.Context, ref *corev1.Reference) (*sloapi.State, error) {
	// return p.storage.Get().SLOState.Get(path.Join("/slo_state", ref.Id))
	return nil, ErrNotImplemented
}
