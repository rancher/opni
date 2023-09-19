package v1

import (
	"time"

	prommodel "github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/validation"
)

var (
	MinEvaluateInterval = time.Minute
)

func (slo *ServiceLevelObjective) Validate() error {
	if slo.Target == nil {
		return validation.Errorf("Target must be set for an SLO")
	}
	if slo.Target.GetValue() < 0 || slo.Target.GetValue() > 100 {
		return validation.Error("objective must be between 0 and 100")
	}
	if slo.GetSloPeriod() == "" {
		return validation.Error("sloPeriod must be set")
	}
	_, err := prommodel.ParseDuration(slo.GetSloPeriod())
	if err != nil {
		return validation.Error("Passed in sloPeriod string is not a valid prometheus duration")
	}
	if slo.GetName() == "" {
		return validation.Error("slo name must be set")
	}
	if slo.GetServiceId() == "" {
		return validation.Error("service must be set")
	}
	if slo.GetGoodMetricName() == "" {
		return validation.Error("goodMetricName must be set")
	}
	if slo.GetTotalMetricName() == "" {
		return validation.Error("totalMetricName must be set")
	}
	if slo.GetClusterId() == "" {
		return validation.Error("clusterId must be set")
	}
	if slo.GetGoodEvents() == nil {
		return validation.Error("goodEvents must always be set for creating an SLO")
	}
	for _, v := range slo.GetGoodEvents() {
		if v.GetKey() == "" {
			return validation.Error("If an event is provided, its key must be set")
		}
		for _, vals := range v.GetVals() {
			if vals == "" {
				return validation.Error("If an event is provided, one of its values cannot be empty")
			}
		}
	}
	interval := slo.GetBudgetingInterval()
	if interval.AsDuration() < MinEvaluateInterval || interval.AsDuration() > time.Hour {
		return validation.Error("budgetingInterval must be between 1 minute and 1 hour")
	}
	if slo.AttachedEndpoints != nil && len(slo.AttachedEndpoints.Items) > 0 {
		if err := slo.AttachedEndpoints.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c *CreateSLORequest) Validate() error {
	slo := c.GetSlo()
	if slo == nil {
		return validation.Error("creating SLO : slo attribute must be set")
	}
	return slo.Validate()
}

func (s *SLOData) Validate() error {
	slo := s.GetSLO()
	if slo == nil {
		return validation.Error("SLO data : slo attribute must be set")
	}
	return s.GetSLO().Validate()
}

func (l *ListServicesRequest) Validate() error {
	if l.GetDatasource() == "" {
		return validation.Error("datasource must be set")
	}
	if l.GetDatasource() != shared.MonitoringDatasource && l.GetDatasource() != shared.LoggingDatasource {
		return shared.ErrInvalidDatasource
	}
	if l.GetClusterId() == "" {
		return validation.Error("clusterId must be set")
	}
	return nil
}

func (l *ListMetricsRequest) Validate() error {
	if l.GetDatasource() == "" {
		return validation.Error("datasource must be set")
	}
	if l.GetDatasource() != shared.MonitoringDatasource && l.GetDatasource() != shared.LoggingDatasource {
		return shared.ErrInvalidDatasource
	}
	if l.GetClusterId() == "" {
		return validation.Error("clusterId must be set")
	}
	if l.GetServiceId() == "" {
		return validation.Error("serviceId must be set")
	}
	return nil
}

func (l *ListEventsRequest) Validate() error {
	if l.GetDatasource() == "" {
		return validation.Error("datasource must be set")
	}
	if l.GetServiceId() == "" {
		return validation.Error("serviceId must be set")
	}
	if l.GetClusterId() == "" {
		return validation.Error("clusterId must be set")
	}
	if l.GetMetricId() == "" {
		return validation.Error("metricId must be set")
	}
	return nil
}
