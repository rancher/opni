package v1

import (
	"slices"
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
	return slices.Contains(svcIds, id)
}

func (e *EventList) ContainsId(id string) bool {
	var eventIds []string
	for _, event := range e.Items {
		eventIds = append(eventIds, event.Key)
	}
	return slices.Contains(eventIds, id)
}
