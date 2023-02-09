package broker_init

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/storage"
	"github.com/rancher/opni/pkg/alerting/storage/jetstream"
	"github.com/rancher/opni/pkg/alerting/storage/mem"
	storage_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
)

const conditionPrefixV1 = "/alerting/conditions"
const endpointPrefixV1 = "/alerting/endpoints"
const statePrefixV1 = "/alerting/state"
const incidentPrefixV1 = "/alerting/incidents"
const routerPrefixV1 = "/alerting/routers"
const defaultTrackerTTLV1 = 24 * time.Hour

func NewDefaultAlertingBroker(js nats.JetStreamContext, opts ...storage_opts.ClientSetOption) storage.AlertingStoreBroker {
	options := &storage_opts.ClientSetOptions{}
	options.Apply(opts...)
	if options.Logger == nil {
		options.Logger = logger.NewPluginLogger().Named("alerting-storage-client-set")
	}
	c := storage.NewCompositeAlertingBroker(*options)
	c.Use(mem.NewInMemoryRouterStore())
	c.Use(
		jetstream.NewJetStreamAlertingStorage[*alertingv1.AlertCondition](
			jetstream.NewConditionKeyStore(js),
			conditionPrefixV1,
		),
	)
	c.Use(
		jetstream.NewJetStreamAlertingStorage[*alertingv1.AlertEndpoint](
			jetstream.NewEndpointKeyStore(js),
			endpointPrefixV1,
		),
	)
	c.Use(
		jetstream.NewJetStreamAlertingStateCache(
			jetstream.NewStatusCache(js),
			statePrefixV1,
		),
	)
	c.Use(
		jetstream.NewJetStreamAlertingIncidentTracker(
			jetstream.NewIncidentKeyStore(js),
			incidentPrefixV1,
			options.TrackerTtl,
		),
	)

	return c
}
