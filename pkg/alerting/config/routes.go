package config

import (
	"fmt"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"golang.org/x/exp/slices"
	"time"
)

func updateRouteWithRequestInfo(route *cfg.Route, req *alertingv1alpha.CreateImplementation) *cfg.Route {
	if req == nil {
		return route
	}
	if req.GetImplementation().GetThrottlingDuration() != nil {
		dur := model.Duration(req.GetImplementation().GetThrottlingDuration().AsDuration())
		route.GroupInterval = &dur

	} else {
		dur := model.Duration(time.Duration(time.Minute * 10))
		route.GroupInterval = &dur
	}

	if req.GetImplementation().GetInitialDelay() != nil {
		dur := model.Duration(req.GetImplementation().GetInitialDelay().AsDuration())
		route.GroupWait = &dur
	} else {
		dur := model.Duration(time.Duration(time.Second * 10))
		route.GroupWait = &dur
	}

	if req.GetImplementation().GetRepeatInterval() != nil {
		dur := model.Duration(req.GetImplementation().GetRepeatInterval().AsDuration())
		route.RepeatInterval = &dur

	} else {
		dur := model.Duration(time.Duration(time.Second * 10))
		route.RepeatInterval = &dur
	}
	return route
}

func (c *ConfigMapData) AppendRoute(recv *Receiver, req *alertingv1alpha.CreateImplementation) {
	r := &cfg.Route{
		Receiver: recv.Name,
		Matchers: cfg.Matchers{
			{
				Name:  shared.BackendConditionIdLabel,
				Value: recv.Name,
			},
		},
	}
	updatedRoute := updateRouteWithRequestInfo(r, req)
	c.Route.Routes = append(c.Route.Routes, updatedRoute)
}

func (c *ConfigMapData) GetRoutes() []*cfg.Route {
	return c.Route.Routes
}

// Assumptions:
// - Id is unique among receivers
// - Route Name corresponds with Ids one-to-one
func (c *ConfigMapData) findRoutes(id string) (int, error) {
	foundIdx := -1
	for idx, r := range c.Route.Routes {
		if r.Receiver == id {
			foundIdx = idx
			break
		}
	}
	if foundIdx < 0 {
		return foundIdx, fmt.Errorf("receiver with id %s not found in alertmanager backend", id)
	}
	return foundIdx, nil
}

func (c *ConfigMapData) UpdateRoute(id string, recv *Receiver, req *alertingv1alpha.CreateImplementation) error {
	if recv == nil {
		return fmt.Errorf("nil receiver passed to UpdateRoute")
	}
	idx, err := c.findRoutes(id)
	if err != nil {
		return err
	}
	r := &cfg.Route{
		Receiver: recv.Name,
		Matchers: cfg.Matchers{
			{
				Name:  shared.BackendConditionIdLabel,
				Value: recv.Name,
			},
		},
	}
	updatedRoute := updateRouteWithRequestInfo(r, req)
	c.Route.Routes[idx] = updatedRoute
	return nil
}

func (c *ConfigMapData) DeleteRoute(id string) error {
	idx, err := c.findRoutes(id)
	if err != nil {
		return err
	}
	c.Route.Routes = slices.Delete(c.Route.Routes, idx, idx+1)
	return nil
}
