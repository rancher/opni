package routing

import alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"

func Equal[T EqualityComparer[T]](a, b T) (bool, string) {
	return a.Equal(b)
}

type EqualityComparer[T any] interface {
	Equal(T) (bool, string)
}

type OpniConfig interface {
	InternalId() string
	ExtractDetails() *alertingv1.EndpointImplementation
	Default() OpniConfig
}

type OpniReceiver interface {
	IsEmpty() bool
}

type OpniRoute[T any] interface {
	EqualityComparer[T]
	NewRouteBase(conditionId string)
	SetGeneralRequestInfo(req *alertingv1.FullAttachedEndpoints)
}

type OpniVirtualRouting interface {
	// when a condition is created
	CreateRoutingNode(conditionId string,
		endpoints *alertingv1.FullAttachedEndpoints)
	// when a condition is updated
	UpdateRoutingNode(conditionId string,
		endpoints *alertingv1.FullAttachedEndpoints)
	// when a condition is deleted
	DeleteRoutingNode(conditionId string)
	// when an endpoint is updated
	UpdateEndpoint(endpointId string)
	// when an endpoint is deleted
	DeleteEndpoint(endpointId string)

	SyncUserConfig(config string) error
	// returns the matched
	WalkUserConfig(labels map[string]string) string
	Construct() *RoutingTree
	From(*RoutingTree) error
}
