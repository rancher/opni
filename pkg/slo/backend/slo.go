package backend

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/samber/lo"
)

// TODO : tests
type SLO struct {
	SloPeriod   string
	Objective   float64 // 0 < x < 100
	Svc         Service
	GoodMetric  Metric
	TotalMetric Metric
	IdLabels    IdentificationLabels
	UserLabels  map[string]string
	GoodEvents  LabelPairs
	TotalEvents LabelPairs
}

type SLODatasource interface {
	SLOStore
	ServiceBackend
	Precondition(ctx context.Context, clusterId *corev1.Reference) error
}

type sLODatasourceImpl struct {
	SLOStore
	ServiceBackend
	precondition func(ctx context.Context, clusterId *corev1.Reference) error
}

func (s *sLODatasourceImpl) Precondition(ctx context.Context, clusterId *corev1.Reference) error {
	return s.precondition(ctx, clusterId)
}

func NewSLODatasource(
	sloStore SLOStore,
	serviceBackend ServiceBackend,
	precondition func(ctx context.Context, clusterId *corev1.Reference) error,
) SLODatasource {
	return &sLODatasourceImpl{
		SLOStore:       sloStore,
		ServiceBackend: serviceBackend,
		precondition:   precondition,
	}
}

func (s *SLO) Validate() error {
	if s.SloPeriod == "" {
		return fmt.Errorf("slo period is required")
	}
	if s.Objective <= 0 || s.Objective >= 100 {
		return fmt.Errorf("slo object must be between 0 and 100 (exclusive)")
	}
	if s.Svc == "" {
		return fmt.Errorf("slo service is required")
	}
	if s.GoodMetric == "" {
		return fmt.Errorf("slo good metric is required")
	}
	if s.TotalMetric == "" {
		return fmt.Errorf("slo total metric is required")
	}
	if len(s.GoodEvents) == 0 {
		return fmt.Errorf("slo good events are required")
	}
	if err := s.IdLabels.Validate(); err != nil {
		return err
	}
	return nil
}

func (s *SLO) GetId() string {
	return s.IdLabels[SLOUuid]
}

func (s *SLO) SetId(id string) {
	s.IdLabels[SLOUuid] = id
}

func (s *SLO) GetName() string {
	return s.IdLabels[SLOName]
}

func (s *SLO) SetName(input string) {
	s.IdLabels[SLOName] = input
}

func (s *SLO) GetPeriod() string {
	return s.SloPeriod
}

func (s *SLO) GetObjective() float64 {
	return s.Objective
}

// MatchEventsOnMetric only applies when the good metric & total metric id is the same
func MatchEventsOnMetric(goodEvents, totalEvents []*slov1.Event) (good, total []*slov1.Event) {
	goodAggregate := map[string][]string{}
	for _, g := range goodEvents {
		if g.Key == "" || g.Vals == nil {
			continue
		}
		if _, ok := goodAggregate[g.Key]; !ok {
			goodAggregate[g.Key] = []string{}
		}
		goodAggregate[g.Key] = append(goodAggregate[g.Key], g.Vals...)
	}
	totalAggregate := map[string][]string{}
	for _, t := range totalEvents {
		if t.Key == "" || t.Vals == nil {
			continue
		}
		if _, ok := totalAggregate[t.Key]; !ok {
			totalAggregate[t.Key] = []string{}
		}
		totalAggregate[t.Key] = append(totalAggregate[t.Key], t.Vals...)
	}
	for key, vals := range totalAggregate {
		if _, ok := goodAggregate[key]; ok {
			// total event filter defined on good and total so reconcile
			totalAggregate[key] = LeftJoinSlice(goodAggregate[key], vals)
		} else {
			// total filter must also apply to good event filter
			goodAggregate[key] = vals
		}
	}
	retGoodEvent := lo.MapToSlice(goodAggregate, func(k string, v []string) *slov1.Event {
		return &slov1.Event{
			Key:  k,
			Vals: v,
		}
	})
	retTotalEvent := lo.MapToSlice(totalAggregate, func(k string, v []string) *slov1.Event {
		return &slov1.Event{
			Key:  k,
			Vals: v,
		}
	})

	slices.SortFunc(retTotalEvent, func(a, b *slov1.Event) int {
		if a.Key < b.Key {
			return -1
		} else if a.Key > b.Key {
			return 1
		}
		return 0
	})

	slices.SortFunc(retGoodEvent, func(a, b *slov1.Event) int {
		if a.Key < b.Key {
			return -1
		} else if a.Key > b.Key {
			return 1
		}
		return 0
	})
	return retGoodEvent, retTotalEvent
}

func CreateSLORequestToStruct(c *slov1.CreateSLORequest) *SLO {
	if c.Slo.GetGoodMetricName() == c.Slo.GetTotalMetricName() {
		c.Slo.GoodEvents, c.Slo.TotalEvents = MatchEventsOnMetric(c.Slo.GoodEvents, c.Slo.TotalEvents)
	}
	reqSLO := c.Slo
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}
	goodEvents := []LabelPair{}
	for _, goodEvent := range reqSLO.GetGoodEvents() {
		goodEvents = append(goodEvents, LabelPair{
			Key:  goodEvent.GetKey(),
			Vals: goodEvent.GetVals(),
		})
	}
	totalEvents := []LabelPair{}
	for _, totalEvent := range reqSLO.GetTotalEvents() {
		totalEvents = append(totalEvents, LabelPair{
			Key:  totalEvent.GetKey(),
			Vals: totalEvent.GetVals(),
		})
	}
	return NewSLO(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		goodEvents,
		totalEvents,
	)
}

func NewSLO(
	sloName string,
	sloPeriod string,
	objective float64,
	svc Service,
	goodMetric Metric,
	totalMetric Metric,
	userLabels map[string]string,
	goodEvents []LabelPair,
	totalEvents []LabelPair,
) *SLO {
	newId := uuid.New().String()
	ilabels := IdentificationLabels{SLOUuid: newId, SLOName: sloName, SLOService: string(svc)}

	return &SLO{
		Svc:         svc,
		SloPeriod:   sloPeriod,
		GoodMetric:  goodMetric,
		TotalMetric: totalMetric,
		UserLabels:  userLabels,
		GoodEvents:  goodEvents,
		TotalEvents: totalEvents,
		IdLabels:    ilabels,
		Objective:   objective,
	}
}

func SLOFromId(
	sloName string,
	sloPeriod string,
	objective float64,
	svc Service,
	goodMetric Metric,
	totalMetric Metric,
	userLabels map[string]string,
	goodEvents []LabelPair,
	totalEvents []LabelPair,
	id string,
) *SLO {
	ilabels := IdentificationLabels{SLOUuid: id, SLOName: sloName, SLOService: string(svc)}

	return &SLO{
		Svc:         svc,
		GoodMetric:  goodMetric,
		TotalMetric: totalMetric,
		SloPeriod:   sloPeriod,
		UserLabels:  userLabels,
		GoodEvents:  goodEvents,
		TotalEvents: totalEvents,
		IdLabels:    ilabels,
		Objective:   objective,
	}
}

func SLODataToStruct(s *slov1.SLOData) *SLO {
	reqSLO := s.SLO
	if reqSLO.GetGoodMetricName() == reqSLO.GetTotalMetricName() {
		reqSLO.GoodEvents, reqSLO.TotalEvents = MatchEventsOnMetric(reqSLO.GoodEvents, reqSLO.TotalEvents)
	}
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}
	goodEvents := []LabelPair{}
	for _, goodEvent := range reqSLO.GetGoodEvents() {
		goodEvents = append(goodEvents, LabelPair{
			Key:  goodEvent.GetKey(),
			Vals: goodEvent.GetVals(),
		})
	}
	totalEvents := []LabelPair{}
	for _, totalEvent := range reqSLO.GetTotalEvents() {
		totalEvents = append(totalEvents, LabelPair{
			Key:  totalEvent.GetKey(),
			Vals: totalEvent.GetVals(),
		})
	}
	if s.Id == "" {
		return NewSLO(
			reqSLO.GetName(),
			reqSLO.GetSloPeriod(),
			reqSLO.GetTarget().GetValue(),
			Service(reqSLO.GetServiceId()),
			Metric(reqSLO.GetGoodMetricName()),
			Metric(reqSLO.GetTotalMetricName()),
			sloLabels,
			goodEvents,
			totalEvents,
		)
	}
	return SLOFromId(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		goodEvents,
		totalEvents,
		s.Id,
	)
}
