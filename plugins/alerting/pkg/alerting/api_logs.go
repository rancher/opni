package alerting

import (
	"context"
	"fmt"
	"sort"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
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
	// persist to disk

	return &empty.Empty{}, nil
}

func (p *Plugin) GetAlertLog(ctx context.Context, ref *corev1.Reference) (*corev1.AlertLog, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertLogs(ctx context.Context, req *alertingv1alpha.ListAlertLogRequest) (*alertingv1alpha.InformativeAlertLogList, error) {
	if p.inMemCache != nil {
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
		// TODO do the actual filtering thing

		return &alertingv1alpha.InformativeAlertLogList{
			Items: toBeFiltered,
		}, nil
	}
	return nil, fmt.Errorf("internal error loading LRU cache")
}

func (p *Plugin) UpdateAlertLog(ctx context.Context, event *alertingv1alpha.UpdateAlertLogRequest) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertLog(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}
