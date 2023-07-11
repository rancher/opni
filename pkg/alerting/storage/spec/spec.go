package spec

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/storage/opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AlertingStoreBroker interface {
	NewClientSet() AlertingClientSet
	// Use applies a specific storage client to the matching storage client in the clientset
	//
	// It is also responsible for migrating existing data if there is an existing store
	// of the same type already in use by the clientset
	Use(store any)
}

type AlertingClientSet interface {
	HashRing
	Purge(ctx context.Context) error
	Conditions() ConditionStorage
	Endpoints() EndpointStorage
	Routers() RouterStorage
	States() StateStorage
	Incidents() IncidentStorage
}

// HashRing Hash ring uniquely maps groups of objects to a (key, hash) pairs
type HashRing interface {
	CalculateHash(ctx context.Context, key string, syncOptions *opts.SyncOptions) error
	GetHash(ctx context.Context, key string) string
	// Sync reads from the client set and updates the router storage in the appropriate way
	// It returns the router keys that have changed.
	Sync(ctx context.Context, opts ...opts.SyncOption) (routerKeys []string, err error)
	ForceSync(ctx context.Context, opts ...opts.SyncOption) error
}

type AlertingStorage[T any] interface {
	Put(ctx context.Context, key string, value T) error
	Get(ctx context.Context, key string, opts ...opts.RequestOption) (T, error)
	Delete(ctx context.Context, key string) error
	ListKeys(ctx context.Context) ([]string, error)
	List(ctx context.Context, opts ...opts.RequestOption) ([]T, error)
}

type AlertingSecretStorage[T interfaces.AlertingSecret] interface {
	AlertingStorage[T]
}

type PartitionedAlertingStorage[T interfaces.AlertingSecret] interface {
	Group(id string) AlertingSecretStorage[T]
	ListGroups(ctx context.Context) ([]string, error)
}

type (
	ConditionStorage = PartitionedAlertingStorage[*alertingv1.AlertCondition]
	EndpointStorage  = AlertingSecretStorage[*alertingv1.AlertEndpoint]
	RouterStorage    = AlertingStorage[routing.OpniRouting]
)

type AlertingStateCache[T interfaces.AlertingSecret] interface {
	AlertingStorage[T]
	IsDiff(ctx context.Context, key string, value T) bool
	LastKnownChange(ctx context.Context, key string) (*timestamppb.Timestamp, error)
}

type StateStorage = AlertingStateCache[*alertingv1.CachedState]

type AlertingIncidentTracker[T interfaces.AlertingSecret] interface {
	AlertingStorage[T]
	OpenInterval(ctx context.Context, conditionId, fingerprint string, start *timestamppb.Timestamp) error
	CloseInterval(ctx context.Context, conditionId, fingerprint string, end *timestamppb.Timestamp) error
	GetActiveWindowsFromIncidentTracker(
		ctx context.Context,
		conditionId string,
		start,
		end *timestamppb.Timestamp,
	) ([]*alertingv1.ActiveWindow, error)
}

type IncidentStorage = AlertingIncidentTracker[*alertingv1.IncidentIntervals]
