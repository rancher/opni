package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	prommodel "github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

const (
	sloRuleNamespace = "slo"
)

type MetricsSLOStoreProvider struct {
	lg          *zap.SugaredLogger
	adminClient cortexadmin.CortexAdminClient
}

func NewMetricsSLOStoreProvider(
	lg *zap.SugaredLogger,
) *MetricsSLOStoreProvider {
	return &MetricsSLOStoreProvider{
		lg: lg,
	}
}

func (m *MetricsSLOStoreProvider) Initialize(adminClient cortexadmin.CortexAdminClient) {
	m.adminClient = adminClient
}

type MetricsSLOStore struct {
	*MetricsSLOStoreProvider

	// backend.ServiceBackend
}

var _ backend.SLOStore = (*MetricsSLOStore)(nil)

func NewMetricsSLOStore(
	provider *MetricsSLOStoreProvider,
) *MetricsSLOStore {
	return &MetricsSLOStore{
		MetricsSLOStoreProvider: provider,
	}
}

func (m *MetricsSLOStore) Create(ctx context.Context, req *slov1.CreateSLORequest) (*corev1.Reference, error) {
	slo := backend.CreateSLORequestToStruct(req)
	sloGen, err := NewSLOGenerator(*slo)
	if err != nil {
		return nil, err
	}
	ruleGroup, err := sloGen.AsRuleGroup()
	if err != nil {
		return nil, err
	}
	yamlBytes, err := yaml.Marshal(ruleGroup)
	if err != nil {
		return nil, err
	}
	if _, err := m.adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		Namespace:   sloRuleNamespace,
		YamlContent: yamlBytes,
		ClusterId:   req.Slo.ClusterId,
	}); err != nil {
		return nil, err
	}

	return &corev1.Reference{
		Id: slo.GetId(),
	}, nil
}
func (m *MetricsSLOStore) Update(ctx context.Context, incoming, existing *slov1.SLOData) (*slov1.SLOData, error) {
	incomingSLO := backend.SLODataToStruct(incoming)
	existingSLO := backend.SLODataToStruct(existing)
	if err := incomingSLO.Validate(); err != nil {
		return nil, err
	}
	if err := existingSLO.Validate(); err != nil {
		return nil, err
	}
	sloGen, err := NewSLOGenerator(*incomingSLO)
	if err != nil {
		return nil, err
	}
	rg, err := sloGen.AsRuleGroup()
	if err != nil {
		return nil, err
	}

	yamlBytes, err := yaml.Marshal(rg)
	if err != nil {
		return nil, err
	}
	if _, err := m.adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		Namespace:   sloRuleNamespace,
		YamlContent: yamlBytes,
		ClusterId:   incoming.SLO.ClusterId,
	}); err != nil {
		return nil, err
	}

	if incoming.SLO.ClusterId != existing.SLO.ClusterId {
		m.adminClient.DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
			Namespace: sloRuleNamespace,
			GroupName: existingSLO.GetId(),
			ClusterId: existing.SLO.ClusterId,
		})
	}
	return incoming, nil
}
func (m *MetricsSLOStore) Delete(ctx context.Context, existing *slov1.SLOData) error {
	slo := backend.SLODataToStruct(existing)
	_, err := m.adminClient.DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
		Namespace: sloRuleNamespace,
		GroupName: slo.GetId(),
		ClusterId: existing.SLO.ClusterId,
	})
	return err
}
func (m *MetricsSLOStore) Clone(ctx context.Context, clone *slov1.SLOData) (*corev1.Reference, *slov1.SLOData, error) {
	clonedData := util.ProtoClone(clone)
	sloData := clonedData.GetSLO()
	clonedSLO := backend.SLODataToStruct(clonedData)
	clonedSLO.SetId(uuid.New().String())
	clonedSLO.SetName(sloData.GetName() + "-clone")
	yamlBytes, err := yaml.Marshal(clonedSLO)
	if err != nil {
		return nil, nil, err
	}
	if _, err := m.adminClient.LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		Namespace:   sloRuleNamespace,
		YamlContent: yamlBytes,
		ClusterId:   sloData.ClusterId,
	}); err != nil {
		return nil, nil, err
	}
	return &corev1.Reference{
		Id: clonedSLO.GetId(),
	}, clonedData, nil
}
func (m *MetricsSLOStore) MultiClusterClone(
	ctx context.Context,
	slo *slov1.SLOData,
	toClusters []*corev1.Reference,
) ([]*corev1.Reference, []*slov1.SLOData, []error) {
	errors := make([]error, len(toClusters))
	cloned := make([]*slov1.SLOData, len(toClusters))
	refs := make([]*corev1.Reference, len(toClusters))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, cluster := range toClusters {
		cluster := cluster
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			clone, _, err := m.Clone(ctx, slo)
			mu.Lock()
			defer mu.Unlock()
			errors[i] = err
			if err != nil {
				return
			}
			cloned[i] = util.ProtoClone(slo)
			cloned[i].SLO.ClusterId = cluster.Id
			refs[i] = clone
		}()
	}
	wg.Wait()
	return refs, cloned, errors

}

// 1. Check if the rule is loaded
// 2. Check SLI, if there is no data, return NoData
// 3. Check the error budget remaining, if exceeded, return Breaching
// 4. Check the MWMB burn rates, if they are elevated, return Warning
// 5. Otherwise, return Ok
func (m *MetricsSLOStore) Status(ctx context.Context, slo *slov1.SLOData) (*slov1.SLOStatus, error) {
	clusterId := slo.SLO.ClusterId
	// 1.
	_, err := m.adminClient.GetRule(ctx, &cortexadmin.GetRuleRequest{
		Namespace: sloRuleNamespace,
		ClusterId: clusterId,
		GroupName: slo.GetId(),
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return &slov1.SLOStatus{
			State: slov1.SLOStatusState_Creating,
		}, nil
	} else if err != nil {
		return nil, err
	}

	sloGen, err := NewSLOGenerator(*backend.SLODataToStruct(slo))
	if err != nil {
		return nil, err
	}

	// 2.
	dur, err := prommodel.ParseDuration(slo.SLO.SloPeriod)
	if err != nil {
		return nil, err
	}
	sliExpr := sloGen.SLI(dur).Expr()
	rawSLIRes, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   sliExpr,
	})
	if err != nil {
		return nil, err
	}
	qrSLI, err := compat.UnmarshalPrometheusResponse(rawSLIRes.Data)
	if err != nil {
		return nil, err
	}
	if len(qrSLI.LinearSamples()) == 0 {
		return &slov1.SLOStatus{
			State: slov1.SLOStatusState_NoData,
		}, nil
	}

	// 3.
	remainingBudget := sloGen.ErrorBudgetRemaining().Expr()
	rawRemBudget, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   remainingBudget,
	})
	if err != nil {
		return nil, err
	}
	qrBudget, err := compat.UnmarshalPrometheusResponse(rawRemBudget.Data)
	if err != nil {
		return nil, err
	}
	samples := qrBudget.LinearSamples()
	if len(samples) == 0 {
		return &slov1.SLOStatus{
			State: slov1.SLOStatusState_NoData,
		}, nil
	}
	lastErrorBudget := samples[len(samples)-1].Value
	if lastErrorBudget < 0 {
		return &slov1.SLOStatus{
			State: slov1.SLOStatusState_Breaching,
		}, nil
	}

	ticketAlert := sloGen.TicketAlert().Expr()
	rawTicketAlert, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   ticketAlert,
	})
	if err != nil {
		return nil, err
	}
	qrTicket, err := compat.UnmarshalPrometheusResponse(rawTicketAlert.Data)
	if err != nil {
		return nil, err
	}
	samples = qrTicket.LinearSamples()
	if len(samples) != 0 {
		lastPageAlert := samples[len(samples)-1].Value
		if lastPageAlert > 0 {
			return &slov1.SLOStatus{
				State: slov1.SLOStatusState_Warning,
			}, nil
		}
	}

	// 4.
	pageAlert := sloGen.PageAlert().Expr()
	rawPageAlert, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   pageAlert,
	})
	if err != nil {
		return nil, err
	}
	qrPage, err := compat.UnmarshalPrometheusResponse(rawPageAlert.Data)
	if err != nil {
		return nil, err
	}
	samples = qrPage.LinearSamples()
	if len(samples) != 0 {
		lastPageAlert := samples[len(samples)-1].Value
		if lastPageAlert > 0 {
			return &slov1.SLOStatus{
				State: slov1.SLOStatusState_Warning,
			}, nil
		}
	}

	return &slov1.SLOStatus{
		State: slov1.SLOStatusState_Ok,
	}, nil
}
func (m *MetricsSLOStore) Preview(ctx context.Context, slo *slov1.CreateSLORequest) (*slov1.SLOPreviewResponse, error) {
	sloDef := backend.SLODataToStruct(&slov1.SLOData{
		Id:  "preview-" + uuid.New().String(),
		SLO: slo.Slo,
	})

	sloGen, err := NewSLOGenerator(*sloDef)
	if err != nil {
		return nil, err
	}

	period, err := prommodel.ParseDuration(slo.Slo.SloPeriod)
	if err != nil {
		return nil, err
	}
	sli := sloGen.SLI(period).Expr()
	cur := time.Now()
	startTs, endTs := cur.Add(time.Duration(-period)), cur
	numSteps := 250
	step := time.Duration(endTs.Sub(startTs).Seconds()/float64(numSteps)) * time.Second
	if step < time.Second {
		step = time.Second
	}
	rawSLI, err := m.adminClient.QueryRange(ctx, &cortexadmin.QueryRangeRequest{
		Tenants: []string{slo.Slo.ClusterId},
		Query:   sli,
		Start:   timestamppb.New(startTs),
		End:     timestamppb.New(endTs),
		Step:    durationpb.New(step),
	})
	if err != nil {
		return nil, err
	}

	qrSLI, err := compat.UnmarshalPrometheusResponse(rawSLI.Data)
	if err != nil {
		return nil, err
	}

	samples := qrSLI.LinearSamples()

	plotVector := &slov1.PlotVector{
		Objective: slo.Slo.Target.Value,
		Items:     make([]*slov1.DataPoint, len(samples)),
	}

	for i := 0; i < len(samples); i++ {
		plotVector.Items[i] = &slov1.DataPoint{
			Sli:       samples[i].Value * 100,
			Timestamp: timestamppb.Now(),
		}
	}

	// TODO: detect alert windows
	// pageAlert := sloGen.PageAlert().Expr()
	// rawPageAlert, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
	// 	Tenants: []string{slo.Slo.ClusterId},
	// 	Query:   pageAlert,
	// })
	// fmt.Println(rawPageAlert)
	// if err != nil {
	// 	return nil, err
	// }
	// // TODO : check values
	// ticketAlert := sloGen.TicketAlert().Expr()
	// rawTicketAlert, err := m.adminClient.Query(ctx, &cortexadmin.QueryRequest{
	// 	Tenants: []string{slo.Slo.ClusterId},
	// 	Query:   ticketAlert,
	// })
	// fmt.Println(rawTicketAlert)
	// if err != nil {
	// 	return nil, err
	// }

	return &slov1.SLOPreviewResponse{
		PlotVector: plotVector,
	}, nil
}
