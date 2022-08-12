package slo

import (
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/validation"
)

func (l *ListServicesRequest) Validate() error {
	if l.GetDatasource() == "" {
		return validation.Error("datasource must be set")
	}
	if l.GetDatasource() != shared.MonitoringDatasource && l.GetDatasource() != shared.LoggingDatasource {
		return shared.ErrInvalidDatasource
	}
	if l.GetClusterdId() == "" {
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
	if l.GetDatasource() != shared.MonitoringDatasource && l.GetDatasource() != shared.LoggingDatasource {
		return shared.ErrInvalidDatasource
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
