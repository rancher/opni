package alerting

import (
	"fmt"

	cfg "github.com/prometheus/alertmanager/config"
	"golang.org/x/exp/slices"
)

func (c *ConfigMapData) AppendRoute(recv *Receiver) {
	c.Route.Routes = append(c.Route.Routes, &cfg.Route{
		Receiver: recv.Name,
		Matchers: cfg.Matchers{
			{
				Name:  "conditionId",
				Value: recv.Name,
			},
		},
	})
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

func (c *ConfigMapData) UpdateRoute(id string, recv *cfg.Route) error {
	if recv == nil {
		return fmt.Errorf("nil receiver passed to UpdateRoute")
	}
	idx, err := c.findRoutes(id)
	if err != nil {
		return err
	}
	c.Route.Routes[idx] = recv
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
