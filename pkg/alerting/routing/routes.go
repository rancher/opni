package routing

import (
	"fmt"
	"time"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/validation"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/shared"
	"golang.org/x/exp/slices"
)

func NewRouteBase(conditionId string) *cfg.Route {
	return &cfg.Route{
		Receiver: conditionId,
		Matchers: cfg.Matchers{
			{
				Name:  shared.BackendConditionIdLabel,
				Value: conditionId,
			},
		},
	}
}

func UpdateRouteWithGeneralRequestInfo(route *cfg.Route, req *alertingv1.FullAttachedEndpoints) error {
	if req == nil {
		return validation.Errorf("cannot pass in nil request to UpdateRouteWithGeneralRequestInfo")
	}
	if dur := req.GetThrottlingDuration(); dur != nil {
		modeldur := model.Duration(dur.AsDuration())
		route.GroupInterval = &modeldur
	} else {
		modeldur := model.Duration(time.Minute * 10)
		route.GroupInterval = &modeldur
	}
	if delay := req.GetInitialDelay(); delay != nil {
		dur := model.Duration(delay.AsDuration())
		route.GroupWait = &dur
	} else {
		dur := model.Duration(time.Second * 10)
		route.GroupWait = &dur
	}
	if rInterval := req.GetRepeatInterval(); rInterval != nil {
		dur := model.Duration(rInterval.AsDuration())
		route.RepeatInterval = &dur
	} else {
		dur := model.Duration(time.Minute * 10)
		route.RepeatInterval = &dur
	}
	return nil
}

func (r *RoutingTree) AppendRoute(updatedRoute *cfg.Route) {
	r.Route.Routes = append(r.Route.Routes, updatedRoute)
}

func (r *RoutingTree) GetRoutes() []*cfg.Route {
	return r.Route.Routes
}

// Assumptions:
// - id is unique among receivers
// - Route Name corresponds with Ids one-to-one
func (r *RoutingTree) FindRoutes(conditionId string) (int, error) {
	foundIdx := -1
	for idx, r := range r.Route.Routes {
		if r.Receiver == conditionId {
			foundIdx = idx
			break
		}
	}
	if foundIdx < 0 {
		return foundIdx, fmt.Errorf("receiver with id %s not found in alertmanager backend", conditionId)
	}
	return foundIdx, nil
}

func (r *RoutingTree) DeleteRoute(conditionId string) error {
	idx, err := r.FindRoutes(conditionId)
	if err != nil {
		return err
	}
	r.Route.Routes = slices.Delete(r.Route.Routes, idx, idx+1)
	return nil
}
