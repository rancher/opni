package storage

import (
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/storage/jetstream"
	"github.com/rancher/opni/pkg/alerting/storage/mem"
	"github.com/rancher/opni/pkg/alerting/storage/opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

var _ RouterStorage = (*jetstream.JetstreamRouterStore[routing.OpniRouting])(nil)
var _ AlertingStorage[interfaces.AlertingSecret] = (*jetstream.JetStreamAlertingStorage[interfaces.AlertingSecret])(nil)
var _ AlertingStateCache[*alertingv1.CachedState] = (*jetstream.JetStreamAlertingStateCache)(nil)
var _ AlertingIncidentTracker[*alertingv1.IncidentIntervals] = (*jetstream.JetStreamAlertingIncidentTracker)(nil)
var _ AlertingStorage[interfaces.AlertingSecret] = (*jetstream.JetStreamAlertingStorage[interfaces.AlertingSecret])(nil)
var _ RouterStorage = (*mem.InMemoryRouterStore)(nil)

type CompositeAlertingBroker struct {
	opts.ClientSetOptions
	*CompositeAlertingClientSet
}

func NewCompositeAlertingBroker(options opts.ClientSetOptions) *CompositeAlertingBroker {
	return &CompositeAlertingBroker{
		ClientSetOptions: options,
		CompositeAlertingClientSet: &CompositeAlertingClientSet{
			hashes: make(map[string]string),
			Logger: options.Logger,
		},
	}
}

var _ AlertingClientSet = (*CompositeAlertingBroker)(nil)
var _ AlertingStoreBroker = (*CompositeAlertingBroker)(nil)

func (c *CompositeAlertingBroker) Use(store any) {
	if cs, ok := store.(ConditionStorage); ok {
		c.conds = cs
	}
	if es, ok := store.(EndpointStorage); ok {
		c.endps = es
	}
	if rs, ok := store.(RouterStorage); ok {
		c.routers = rs
	}
	if ss, ok := store.(StateStorage); ok {
		c.states = ss
	}
	if is, ok := store.(IncidentStorage); ok {
		c.incidents = is
	}
}

func (c *CompositeAlertingBroker) NewClientSet() AlertingClientSet {
	return c.CompositeAlertingClientSet
}
