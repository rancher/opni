/// Shared datatypes for downstream service discovery

/// There are several ways to do downstream service discovery:
/// 1. Prometheus comes with a built in auto-discovery:
/// 2. Have a custom agent that uses kubernetes built in to monitor pod creation and deletion
/// 3. Monitor/ User create ConfigMaps to define the downstream services

package discovery

import (
	"context"
	"errors"
)

type DiscoveryRule func(ctx context.Context) error

type Discoverable interface {
	Discover(ctx context.Context) error
}

type ServiceGroup struct {
	Services []Discoverable
}

/// Service base class
// TODO: refactor to interface
type Service struct {
	clusterId   string
	serviceName string
	serviceId   string
}

func NewService(clusterId string, serviceName string, serviceId string) *Service {
	return &Service{
		clusterId:   clusterId,
		serviceName: serviceName,
		serviceId:   serviceId,
	}
}

/// Returns whether or not this is a "discrete" kubernetes object
/// "discrete" kubernetes objects should not be monitored by the auto-discovery,
/// rather whatever controls the orchestration should be monitored
func (s *Service) IsServiceMonomer() (bool, error) {
	return false, errors.New("not implemeted")
}

/// Returns whether this orchestrates a group of "discrete" kubernetes objects
func (s *Service) IsServicePolymer() (bool, error) {
	return false, errors.New("not implemeted")
}

func (s *Service) GetParentOrchestrator() (*Service, error) {
	return nil, errors.New("not implemeted")
}

func (s *Service) GetClusterId() string {
	return s.clusterId
}

func (s *Service) GetServiceName() string {
	return s.serviceName
}

func (s *Service) GetServiceId() string {
	return s.serviceId
}

func (s *Service) SetServiceName(serviceName string) {
	s.serviceName = serviceName
}

func (s *Service) SetServiceId(serviceId string) {
	s.serviceId = serviceId
}

func (s *Service) Discover(ctx context.Context) error {
	return errors.New("not implemeted")
}
