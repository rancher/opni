/* API implementation
 */
package slo

import (
	"context"
	"fmt"
	"path"

	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
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

// Cortex applies rule groups individually
func applyCortexSLORules(p *Plugin, cortexRules *CortexRuleWrapper, service *sloapi.Service, existingId string, ctx context.Context, lg hclog.Logger) error {
	var anyError error
	_, err := p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.recording,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load recording rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.metadata,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load metadata rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.YamlRequest{
		Yaml:   cortexRules.alerts,
		Tenant: service.ClusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load alerting rules for cluster %s, service %s, id %s : %v",
			service.ClusterId, service.JobId, existingId, anyError))
		anyError = err
	}
	return anyError
}

func deleteCortexSLORules(p *Plugin, toDelete *sloapi.SLOImplData, ctx context.Context, lg hclog.Logger) error {
	id, clusterId := toDelete.Id, toDelete.Service.ClusterId
	var anyError error
	_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + RecordingRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete recording rule group with id  %v: %v", id, err))
		anyError = err
	}
	_, err = p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + MetadataRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete metadata rule group with id  %v: %v", id, err))
		anyError = err
	}
	_, err = p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		Tenant:    clusterId,
		GroupName: id + AlertRuleSuffix,
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to delete alerting rule group with id  %v: %v", id, err))
		anyError = err
	}
	return anyError
}

// Convert OpenSLO specs to Cortex Rule Groups & apply them
func applyMonitoringSLODownstream(osloSpec oslov1.SLO, service *sloapi.Service, existingId string,
	p *Plugin, slorequest *sloapi.CreateSLORequest, ctx context.Context, lg hclog.Logger) ([]*sloapi.SLOImplData, error) {
	slogroup, err := ParseToPrometheusModel(osloSpec)
	if err != nil {
		lg.Error("failed to parse prometheus model IR :", err)
		return nil, err
	}

	returnedSloImpl := []*sloapi.SLOImplData{}
	rw, err := GeneratePrometheusNoSlothGenerator(slogroup, ctx, lg)
	if err != nil {
		lg.Error("Failed to generate prometheus : ", err)
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Generated cortex rule groups : %d", len(rw)))
	if len(rw) > 1 {
		lg.Warn("Multiple cortex rule groups being applied")
	}
	for _, rwgroup := range rw {
		// Generate new uuid for new slo
		if existingId == "" {
			existingId = uuid.New().String()
		}

		cortexRules, err := toCortexRequest(rwgroup, existingId)
		if err != nil {
			return nil, err
		}
		applyCortexSLORules(p, cortexRules, service, existingId, ctx, lg)

		dataToPersist := &sloapi.SLOImplData{
			Id:      existingId,
			SLO:     slorequest.SLO,
			Service: service,
		}
		returnedSloImpl = append(returnedSloImpl, dataToPersist)
	}
	return returnedSloImpl, nil
}

func (p *Plugin) CreateSLO(ctx context.Context, slorequest *sloapi.CreateSLORequest) (*sloapi.CreatedSLOs, error) {
	lg := p.logger
	if err := ValidateInput(slorequest); err != nil {
		return nil, err
	}
	osloSpecs, err := ParseToOpenSLO(slorequest, ctx, p.logger)
	if err != nil {
		return nil, err
	}
	lg.Debug(fmt.Sprintf("Length of osloSpecs : %d", len(osloSpecs)))

	switch slorequest.SLO.GetDatasource() {
	case shared.LoggingDatasource:
		return nil, shared.ErrNotImplemented
	case shared.MonitoringDatasource:
		returnedSloId := &sloapi.CreatedSLOs{}
		openSpecServices, err := zipOpenSLOWithServices(osloSpecs, slorequest.Services)
		if err != nil {
			return nil, err
		}
		// possible for partial success, but don't want to exit on error
		var anyError error
		for _, zipped := range openSpecServices {
			// existingId="" if this is a new slo
			createdSlos, err := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service, "", p, slorequest, ctx, lg)

			if err != nil {
				anyError = err
			}
			for _, data := range createdSlos {
				returnedSloId.Items = append(returnedSloId.Items, &corev1.Reference{Id: data.Id})
				if err := p.storage.Get().SLOs.Put(path.Join("/slos", data.Id), data); err != nil {
					return nil, err
				}
				if err != nil {
					anyError = err
				}
			}
		}
		return returnedSloId, anyError
	default:
		return nil, status.Error(codes.InvalidArgument, "Invalid datasource")
	}
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

func (p *Plugin) UpdateSLO(ctx context.Context, req *sloapi.SLOImplData) (*emptypb.Empty, error) {
	lg := p.logger
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}
	createReq := &sloapi.CreateSLORequest{
		SLO:      req.SLO,
		Services: []*sloapi.Service{req.Service},
	}
	osloSpecs, err := ParseToOpenSLO(createReq, ctx, lg)
	if err != nil {
		return nil, err
	}
	// possible for partial success, but don't want to exit on error
	var anyError error
	switch req.SLO.GetDatasource() {
	case shared.MonitoringDatasource:
		openSpecServices, err := zipOpenSLOWithServices(osloSpecs, []*sloapi.Service{req.Service})
		if err != nil {
			return nil, err
		}
		// changing clusters means we need to clean up the rules on the old cluster
		if existing.Service.ClusterId != req.Service.ClusterId {
			p.DeleteSLO(ctx, &corev1.Reference{Id: req.Id})
		}
		for _, zipped := range openSpecServices {
			// don't need creation metadata
			_, err := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service, req.Id, p, createReq, ctx, lg)

			if err != nil {
				anyError = err
			}
		}
	case shared.LoggingDatasource:
		return nil, shared.ErrNotImplemented
	default:
		return nil, status.Error(codes.InvalidArgument, "Invalid datasource")
	}

	// Merge when everything else is done
	proto.Merge(existing, req)
	if err := p.storage.Get().SLOs.Put(path.Join("/slos", req.Id), existing); err != nil {
		return nil, err
	}
	lg.Debug("Merge successful")
	// need this because retruning anyError along with empty protobuf cause it
	// to be intercepted as dynamic message type, causing a panic
	if anyError != nil {
		return nil, anyError
	}
	lg.Debug("Update successful")
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteSLO(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.logger
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", req.Id))
	if err != nil {
		return nil, err
	}

	switch existing.SLO.GetDatasource() {
	case shared.LoggingDatasource:
		return nil, shared.ErrNotImplemented
	case shared.MonitoringDatasource:
		// delete the rule groups that make up the SLO
		err := deleteCortexSLORules(p, existing, ctx, lg)
		if err != nil {
			return nil, err
		}
		if err := p.storage.Get().SLOs.Delete(path.Join("/slos", req.Id)); err != nil {
			return nil, err
		}
		// delete if found
		p.storage.Get().SLOs.Delete(path.Join("/slo_state", req.Id))
		return &emptypb.Empty{}, err
	default:
		return nil, status.Error(codes.InvalidArgument, "Invalid datasource")
	}
}

func (p *Plugin) CloneSLO(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOImplData, error) {
	existing, err := p.storage.Get().SLOs.Get(path.Join("/slos", ref.Id))
	if err != nil {
		return nil, err
	}

	clone := proto.Clone(existing).(*sloapi.SLOImplData)
	clone.Id = ""
	clone.SLO.Name = clone.SLO.Name + " - Copy"
	var anyError error
	switch clone.SLO.GetDatasource() {
	case shared.MonitoringDatasource:
		// create the slo
		createdSlos, err := p.CreateSLO(ctx, &sloapi.CreateSLORequest{
			SLO:      clone.SLO,
			Services: []*sloapi.Service{clone.Service},
		})
		// should only create one slo
		if len(createdSlos.Items) > 1 {
			anyError = status.Error(codes.Internal, "Created more than one SLO")
		}
		clone.Id = createdSlos.Items[0].Id
		if err := p.storage.Get().SLOs.Put(path.Join("/slos", createdSlos.Items[0].Id), clone); err != nil {
			return nil, err
		}
		if err != nil {
			anyError = err
		}

	case shared.LoggingDatasource:
		return nil, shared.ErrNotImplemented
	default:
		return nil, status.Error(codes.InvalidArgument, "Invalid datasource")
	}
	return clone, anyError
}

func (p *Plugin) Status(ctx context.Context, ref *corev1.Reference) (*sloapi.SLOStatus, error) {
	return nil, shared.ErrNotImplemented
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

func (p *Plugin) GetFormulas(ctx context.Context, ref *corev1.Reference) (*sloapi.Formula, error) {
	// return p.storage.Get().Formulas.Get(path.Join("/formulas", ref.Id))
	return nil, shared.ErrNotImplemented
}

func (p *Plugin) ListFormulas(ctx context.Context, _ *emptypb.Empty) (*sloapi.FormulaList, error) {
	items, err := list(p.storage.Get().Formulas, "/formulas")
	if err != nil {
		return nil, err
	}
	return &sloapi.FormulaList{
		Items: items,
	}, shared.ErrNotImplemented
}

func (p *Plugin) SetState(ctx context.Context, req *sloapi.SetStateRequest) (*emptypb.Empty, error) {
	// if err := p.storage.Get().SLOState.Put(path.Join("/slo_state", req.Slo.Id), req.State); err != nil {
	// 	return nil, err
	// }
	return &emptypb.Empty{}, shared.ErrNotImplemented

}

func (p *Plugin) GetState(ctx context.Context, ref *corev1.Reference) (*sloapi.State, error) {
	// return p.storage.Get().SLOState.Get(path.Join("/slo_state", ref.Id))
	return nil, shared.ErrNotImplemented
}
