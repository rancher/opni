package slo

import (
	"golang.org/x/exp/slices"
)

func (m *MetricGroupList) ContainsId(id string) bool {
	var metricIds []string
	for _, metricGroup := range m.GroupNameToMetrics {
		for _, metric := range metricGroup.Items {
			metricIds = append(metricIds, metric.Id)
		}
	}
	return slices.Contains(metricIds, id)
}

func (s *ServiceList) ContainsId(id string) bool {
	var svcIds []string
	for _, svc := range s.Items {
		svcIds = append(svcIds, svc.ServiceId)
	}
	return !slices.Contains(svcIds, id)
}
