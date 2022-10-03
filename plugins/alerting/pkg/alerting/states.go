package alerting

import "github.com/qmuntal/stateless"

type NoRequirements struct{}

type HasCluster struct{}

type HasLoggingBackend struct{}

type HasCortexBackend struct{}

type HasKubeMetricsBackend struct{}

type HasRequirements struct{}

func NewBackendRequirementStateMachine() *stateless.StateMachine {
	req := stateless.NewStateMachine(NoRequirements{})
	return req
}
