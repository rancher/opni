package storage

import (
	"fmt"
	"regexp"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/storage/jetstream"
	"github.com/rancher/opni/pkg/alerting/storage/mem"
	storage_opts "github.com/rancher/opni/pkg/alerting/storage/opts"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
)

var (
	groupPrefixV1 = func(groupId string) string {
		return fmt.Sprintf("/alerting/%s/conditions", groupId)
	}
	grouptRe = regexp.MustCompile(`^/alerting/(.+)/conditions`)
)

const (
	conditionPrefixV1   = "/alerting/conditions"
	endpointPrefixV1    = "/alerting/endpoints"
	statePrefixV1       = "/alerting/state"
	incidentPrefixV1    = "/alerting/incidents"
	routerPrefixV1      = "/alerting/routers"
	defaultTrackerTTLV1 = 24 * time.Hour
)

func NewDefaultAlertingBroker(js nats.JetStreamContext, opts ...storage_opts.ClientSetOption) spec.AlertingStoreBroker {
	options := &storage_opts.ClientSetOptions{}
	options.Apply(opts...)
	if options.Logger == nil {
		options.Logger = logger.NewPluginLogger().WithGroup("alerting-storage-client-set")
	}
	if options.TrackerTtl == 0 {
		options.TrackerTtl = defaultTrackerTTLV1
	}
	c := NewCompositeAlertingBroker(*options)
	c.Use(mem.NewInMemoryRouterStore())
	c.Use(
		jetstream.NewConditionGroupManager[*alertingv1.AlertCondition](
			jetstream.NewConditionKeyStore(js),
			conditionPrefixV1,
			groupPrefixV1,
			grouptRe,
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
