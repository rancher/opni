package jetstream

import (
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/util"
)

func NewRouterObjectStore(js nats.JetStreamContext) nats.ObjectStore {
	return util.Must(js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      shared.RouterStorage,
		Description: "track constructed and synced routing trees",
		Storage:     nats.FileStorage,
	}))
}

func NewIncidentKeyStore(js nats.JetStreamContext) nats.KeyValue {
	return util.Must(js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      shared.GeneralIncidentStorage,
		Description: "track internal incident changes over time for each condition id",
		Storage:     nats.FileStorage,
	}))
}

func NewStatusCache(js nats.JetStreamContext) nats.KeyValue {
	return util.Must(js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      shared.StatusBucketPerCondition,
		Description: "track last known internal status for each condition id",
		Storage:     nats.FileStorage,
	}))
}

func NewConditionKeyStore(js nats.JetStreamContext) nats.KeyValue {
	return util.Must(js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      shared.AlertingConditionBucket,
		Description: "track last known internal status for each condition id",
		Storage:     nats.FileStorage,
	}))
}

func NewEndpointKeyStore(js nats.JetStreamContext) nats.KeyValue {
	return util.Must(js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      shared.AlertingEndpointBucket,
		Description: "track last known internal status for each condition id",
		Storage:     nats.FileStorage,
	}))
}
