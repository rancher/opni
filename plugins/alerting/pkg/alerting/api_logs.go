package alerting

import (
	"context"
	"fmt"
	"sort"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type By func(i1, i2 *alertingv1alpha.InformativeAlertLog) bool

type alertInfoSorter struct {
	info []*alertingv1alpha.InformativeAlertLog
	by   func(i1, i2 *alertingv1alpha.InformativeAlertLog) bool // Closure used in the Less method.
}

func (by By) Sort(alerts []*alertingv1alpha.InformativeAlertLog) {
	alertInfoSorter := &alertInfoSorter{
		info: alerts,
		by:   by,
	}
	sort.Sort(alertInfoSorter)
}

// implement sort interface
func (a *alertInfoSorter) Len() int {
	return len(a.info)
}

func (a *alertInfoSorter) Swap(i, j int) {
	a.info[i], a.info[j] = a.info[j], a.info[i]
}

func (a *alertInfoSorter) Less(i, j int) bool {
	return a.by(a.info[i], a.info[j])
}

func (p *Plugin) CreateAlertLog(ctx context.Context, event *corev1.AlertLog) (*emptypb.Empty, error) {
	if p.inMemCache != nil {
		p.inMemCache.Add(event.ConditionId, event)
	}
	existing, err := GetIndices() // get the existing indices for each condition
	if err != nil {
		return nil, err
	}
	found := false
	var b *BucketInfo
	for _, index := range existing { // check if condition is already indexed
		if index.ConditionId == event.ConditionId.Id {
			found = true
			b = index
			break
		}
	}
	if found { // append to most recent bucket in existing index
		if err := b.Append(event); err != nil {
			return nil, err
		}
	} else {
		if b == nil {
			b = &BucketInfo{
				ConditionId: event.ConditionId.Id,
			}
		}
		if err := b.Create(); err != nil {
			return nil, err
		}
		if err := b.Append(event); err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

// https://stackoverflow.com/questions/18879109/subset-check-with-slices-in-go
func isSubset(first, second []string) bool {
	set := make(map[string]int)
	for _, value := range second {
		set[value] += 1
	}

	for _, value := range first {
		if count, found := set[value]; !found {
			return false
		} else if count < 1 {
			return false
		} else {
			set[value] = count - 1
		}
	}
	return true
}

func hasLabels(item *alertingv1alpha.InformativeAlertLog, labels []string) bool {
	return len(labels) == 0 || isSubset(labels, item.Condition.Labels)
}

// rounds to the nearest second, so need to check for equality
func hasRange(item *alertingv1alpha.InformativeAlertLog, s *timestamppb.Timestamp, e *timestamppb.Timestamp) bool {
	if item.Log.Timestamp == nil { // shouldn't happen, but hey
		return true
	}
	start := s.AsTime()
	end := e.AsTime()
	if s == nil && e == nil {
		return true
	} else if s == nil { // before end
		t := item.Log.Timestamp.AsTime()
		return t == end || t.Before(end)
	} else if e == nil {
		t := item.Log.Timestamp.AsTime()
		return t == start || t.After(start)
	} else {
		t := item.Log.Timestamp.AsTime()
		return (t == start || t.After(start)) && (t == end || t.Before(end))
	}
}

func (p *Plugin) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*alertingv1alpha.InformativeAlertLogList, error) {
	if p.inMemCache == nil {
		return nil, fmt.Errorf("internal error loading LRU cache")
	}
	conditionsFrequentlyFired := make([]*corev1.AlertLog, 0)
	keys := p.inMemCache.Keys()
	for _, key := range keys {
		val, _ := p.inMemCache.Peek(key)
		conditionsFrequentlyFired = append(conditionsFrequentlyFired, val.(*corev1.AlertLog))
	}
	toBeFiltered := []*alertingv1alpha.InformativeAlertLog{}
	for _, log := range conditionsFrequentlyFired {
		var cnd *alertingv1alpha.AlertCondition
		var err error
		cnd, err = p.GetAlertCondition(ctx, log.ConditionId)
		if err != nil {
			// template an alert condition
			cnd = &alertingv1alpha.AlertCondition{
				Name:        "<not found>",
				Description: "<not found>",
			}
		}
		toBeFiltered = append(toBeFiltered, &alertingv1alpha.InformativeAlertLog{
			Condition: cnd,
			Log:       log,
		})
	}
	var toBeReturned []*alertingv1alpha.InformativeAlertLog
	for _, item := range toBeFiltered {
		if hasLabels(item, req.Labels) && hasRange(item, req.StartTimestamp, req.EndTimestamp) {
			toBeReturned = append(toBeReturned, item)
		}
	}

	return &alertingv1alpha.InformativeAlertLogList{
		Items: toBeReturned,
	}, nil
}
